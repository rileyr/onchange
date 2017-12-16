package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

func main() {
	RootCmd.Execute()
}

var log *logrus.Logger

var RootCmd = &cobra.Command{
	Use:              "onchange",
	Short:            "file change command runner",
	Long:             longDesc,
	PersistentPreRun: setLogger,
	PreRunE:          validateArgs,
	RunE:             runOnchange,
}

func init() {
	RootCmd.PersistentFlags().StringP("watch-dir", "d", "", "directory to watch")
	RootCmd.PersistentFlags().StringP("command", "c", "", "command to run")
	RootCmd.PersistentFlags().StringP("exclude", "e", "", "exclude pattern")
	RootCmd.PersistentFlags().StringP("interval", "i", "1000ms", "check interval (ms/ns)")
	RootCmd.PersistentFlags().BoolP("verbose-log", "v", false, "enable verbose logging")
}

func setLogger(c *cobra.Command, args []string) {
	log = logrus.New()

	if debug, _ := c.Flags().GetBool("verbose-log"); debug {
		log.SetLevel(logrus.DebugLevel)
		log.Debugln("verbose logging enabled")
	} else {
		log.SetLevel(logrus.InfoLevel)
	}
}

func validateArgs(c *cobra.Command, args []string) error {
	if d, _ := c.Flags().GetString("watch-dir"); d == "" {
		return errors.New("watch-dir is required!")
	}

	if c, _ := c.Flags().GetString("command"); c == "" {
		return errors.New("command is required!")
	}

	intStr, _ := c.Flags().GetString("interval")
	if intStr != "" {
		if !strings.Contains(intStr, "ns") && !strings.Contains(intStr, "ms") {
			return fmt.Errorf("unknown interval: %s", intStr)
		}
	}

	return nil
}

func runOnchange(c *cobra.Command, args []string) error {
	cmd, _ := c.Flags().GetString("command")
	dir, _ := c.Flags().GetString("watch-dir")
	intStr, _ := c.Flags().GetString("interval")
	ex, _ := c.Flags().GetString("exclude")

	var dur time.Duration

	if strings.Contains(intStr, "ns") {
		n := strings.Replace(intStr, "ns", "", -1)
		i, _ := strconv.Atoi(n)
		dur = time.Nanosecond * time.Duration(i)
	} else {
		n := strings.Replace(intStr, "ms", "", -1)
		i, _ := strconv.Atoi(n)
		dur = time.Millisecond * time.Duration(i)
	}

	r := &runner{
		watchDir:    dir,
		cmdStr:      cmd,
		resetTicker: time.NewTicker(dur),
		resetNext:   true,
		mu:          &sync.Mutex{},
	}

	exArr := []string{".git"}
	if ex != "" {
		arr := strings.Split(ex, ",")
		for _, e := range arr {
			exArr = append(exArr, e)
		}
	}
	r.ex = exArr

	log.Debugf("starting: %#v", r)
	return r.Run()
}

type runner struct {
	// watchDir is the directory to watch; could be relative or absolute.
	watchDir string

	// cmdStr is the command to execute on file change.
	cmdStr string

	// resetTicker is the ticker that controls checking the restart flag.
	resetTicker *time.Ticker

	// resetNext is the flag that informs the runner if a reset is needed on next tick.
	resetNext bool

	// ex are patterns to exclude
	ex []string

	mu *sync.Mutex
}

// Run is the main logic that runs the onChange app.
// The core for/select statement handles the following events:
//
//	- fsnotify.Event: any event that should trigger a restart should set the "shouldRestart"
//										boolean on the watcher, so that the tick-checker restarts the application on the next tick.
//
//	-	resetTicker: a ticker that checks the flag, and executes a reset if it's been set.
//
//	- fsnotify.Error: reports the error and exits the program.
//
//
func (r *runner) Run() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	filepath.Walk(r.watchDir, func(p string, i os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !i.IsDir() {
			return nil
		}

		if r.exclude(p) {
			return nil
		}

		log.Debugf("watching %s", p)
		return w.Add(p)
	})
	if err := w.Add(r.watchDir); err != nil {
		return err
	}

	var cmd *exec.Cmd
	var done = make(chan error)

	for {
		select {
		case err := <-done:
			if err != nil && err.Error() != "signal: killed" {
				log.Error(err)
			}
		case <-r.resetTicker.C:
			r.mu.Lock()
			if r.resetNext {
				log.Infof("running command: %s", r.cmdStr)
				r.resetNext = false

				if cmd != nil {
					log.Debugf("killing current process")
					err := cmd.Process.Kill()
					if err != nil && err.Error() != "os: process already finished" {
						return err
					}
					cmd = nil
				}

				cmd = r.newCmd()
				if err := cmd.Start(); err != nil {
					return err
				}
				go func() {
					done <- cmd.Wait()
				}()
			}
			r.mu.Unlock()
		case e := <-w.Events:
			if e.Op == fsnotify.Chmod || r.exclude(e.String()) {
				log.Debugf("skipping %s", e.String())
			} else {
				log.Debugf("got event: %s", e.String())
				r.mu.Lock()
				r.resetNext = true
				r.mu.Unlock()
			}
		case err := <-w.Errors:
			return err
		}
	}

	return nil
}

func (r *runner) newCmd() *exec.Cmd {
	cmdArgs := strings.Split(r.cmdStr, " ")
	c := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	c.Stdout = os.Stdout
	return c
}

const longDesc = `
 ____ ____ ____ ____ ____ ____ ____ ____ 
||o |||n |||c |||h |||a |||n |||g |||e ||
||__|||__|||__|||__|||__|||__|||__|||__||
|/__\|/__\|/__\|/__\|/__\|/__\|/__\|/__\|

runs a command. when in the given dir changes, kill the old command if it's still running, and then run it again
`

func (r *runner) exclude(p string) bool {
	if len(r.ex) < 1 {
		return false
	}

	for _, e := range r.ex {
		if strings.Contains(p, e) {
			return true
		}
	}

	return false
}
