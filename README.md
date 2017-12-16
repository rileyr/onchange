# onchange

quick and dirty command runner

```shell

 ____ ____ ____ ____ ____ ____ ____ ____
||o |||n |||c |||h |||a |||n |||g |||e ||
||__|||__|||__|||__|||__|||__|||__|||__||
|/__\|/__\|/__\|/__\|/__\|/__\|/__\|/__\|

runs a command. when in the given dir changes, kill the old command if it's still running, and then run it again

Usage:
  onchange [flags]

Flags:
  -c, --command string     command to run
  -h, --help               help for onchange
  -i, --interval string    check interval (ms/ns) (default "1000ms")
  -v, --verbose-log        enable verbose logging
  -d, --watch-dir string   directory to watch
```

---

onchange watches a directory for file changes, and runs a given command when something happens. internally, onchange uses a polling mechanism to nicely handle text editors that make many updates to multiple files when a single file is changed.

example:

```shell
/onchange $ go run main.go -d=. -c="ruby foo.rb"
INFO[0001] running command: ruby foo.rb
Starting!
0
1
2
3
4
INFO[0006] running command: ruby foo.rb
Starting!
0
1
2
3
4
5
6
7
8
9
10
```
