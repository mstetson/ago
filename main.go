package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"9fans.net/go/acme"
)

var dir string

var installAfter = flag.Bool("i", false, "run go install ./cmd/... after tests pass")
var runAfter = flag.String("r", "", "run command after every successful build")

func main() {
	flag.Parse()
	dir = os.Getenv("%")
	if !strings.HasSuffix(dir, "/") {
		dir = filepath.Dir(dir)
	}
	os.Setenv("PWD", "") // Removing the PWD kludge helps the go tools find GOPATH.
	win, err := acme.New()
	if err != nil {
		log.Fatal(err)
	}
	err = win.Name(filepath.Join(dir, "+Ago"))
	if err != nil {
		log.Fatal(err)
	}
	err = win.Ctl("clean")
	if err != nil {
		log.Fatal(err)
	}

	go processEvents(win)

	out := &output{w: win}

	l, err := acme.Log()
	if err != nil {
		log.Fatal(err)
	}

	for {
		event, err := l.Read()
		if err != nil {
			log.Fatal(err)
		}
		if event.Name == "" || event.Op != "put" || !strings.HasPrefix(event.Name, dir) {
			continue
		}
		if strings.HasSuffix(event.Name, ".go") {
			putGo(out, event.ID, event.Name)
		}
	}
}

func processEvents(win *acme.Win) {
	for e := range win.EventChan() {
		switch e.C2 {
		case 'x', 'X': // execute
			if string(e.Text) == "Del" {
				win.Ctl("delete")
			}
		}
		win.WriteEvent(e)
	}
	os.Exit(0)
}

func putGo(out *output, winID int, name string) {
	clear(out)
	defer dollar(out)
	if !gofmt(out, winID, name) {
		return
	}
	relPkg := relativePackage(name)
	if relPkg == "" {
		return
	}
	if !run(out, "go", "test", relPkg) {
		return
	}
	if *installAfter && !run(out, "go", "install", "./cmd/...") {
		return
	}
	if *runAfter != "" && !runRc(out, *runAfter) {
		return
	}
}

func relativePackage(name string) string {
	d := filepath.Dir(name)
	rel, err := filepath.Rel(dir, d)
	if err != nil {
		return ""
	}
	if rel == "" {
		return "."
	}
	return "./" + rel
}

func clear(out *output) {
	err := out.Clear()
	if err != nil {
		log.Fatal(err)
	}
}

func dollar(out *output) {
	_, err := fmt.Fprintln(out, "$")
	if err != nil {
		log.Fatal(err)
	}
}

type output struct {
	sync.Mutex
	w *acme.Win
}

func (o *output) Clear() error {
	o.Lock()
	defer o.Unlock()
	err := o.w.Addr(",")
	if err != nil {
		return err
	}
	_, err = o.w.Write("data", nil)
	if err != nil {
		return err
	}
	return o.w.Ctl("clean")
}

func (o *output) Write(b []byte) (int, error) {
	o.Lock()
	defer o.Unlock()
	bc, nb := stripTerminal(b)
	n, err := o.w.Write("body", bc)
	if err != nil {
		return n + nb, err
	}
	err = o.w.Ctl("clean")
	return n + nb, err
}

// This re matches common ANSI control sequences and any single character followed by backspace.
var terminalRE = regexp.MustCompile(".\b|\033\\[[0-9;\\?]+[A-Za-z]")

func stripTerminal(b []byte) ([]byte, int) {
	s := terminalRE.ReplaceAll(b, nil)
	return s, len(b) - len(s)
}

func run(out *output, name string, arg ...string) bool {
	_, err := fmt.Fprintf(out, "$ %s %s\n", name, strings.Join(arg, " "))
	if err != nil {
		log.Fatal(err)
	}
	cmd := exec.Command(name, arg...)
	cmd.Stdout = out
	cmd.Stderr = out
	err = cmd.Run()
	if err != nil {
		_, err = fmt.Fprintf(out, "%s %s: %s\n", name, strings.Join(arg, " "), err)
		if err != nil {
			log.Fatal(err)
		}
		return false
	}
	return true
}

func runRc(out *output, rcCmd string) bool {
	_, err := fmt.Fprintf(out, "$ %s\n", rcCmd)
	if err != nil {
		log.Fatal(err)
	}
	cmd := exec.Command("rc", "-c", rcCmd)
	cmd.Stdout = out
	cmd.Stderr = out
	err = cmd.Run()
	if err != nil {
		_, err = fmt.Fprintf(out, "%s: %s\n", rcCmd, err)
		if err != nil {
			log.Fatal(err)
		}
		return false
	}
	return true
}
