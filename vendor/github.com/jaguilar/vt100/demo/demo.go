package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/facebookgo/httpdown"
	"github.com/golang/glog"
	"github.com/jaguilar/vt100"
	vtexport "github.com/jaguilar/vt100/export"
	"github.com/kr/pty"
	"github.com/pkg/term/termios"
)

var (
	port int
)

func init() {
	flag.IntVar(&port, "port", 0, "open a debug server on port")
}

func tcgetattr(fd uintptr) (syscall.Termios, error) {
	var t syscall.Termios
	if err := termios.Tcgetattr(fd, &t); err != nil {
		return t, fmt.Errorf("tcgetattr: %v", err)
	}
	return t, nil
}

func tcsetattr(fd uintptr, t syscall.Termios) error {
	if err := termios.Tcsetattr(fd, termios.TCSAFLUSH, &t); err != nil {
		return fmt.Errorf("tcsetattr: %v", err)
	}
	return nil
}

func makeRaw(fd uintptr) (syscall.Termios, error) {
	orig, err := tcgetattr(fd)
	if err != nil {
		return orig, err
	}

	t := orig
	termios.Cfmakeraw(&t)
	if err := tcsetattr(fd, t); err != nil {
		return orig, err
	}
	return orig, nil
}

type vt struct {
	*vt100.VT100
	sync.Locker
}

func main() {
	flag.Parse()

	origAttr, err := makeRaw(os.Stdout.Fd())
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := tcsetattr(os.Stdout.Fd(), origAttr); err != nil {
			glog.Error(err) // Nothing much we can do about this error.
		}
	}()

	c := exec.Command("nethack")
	c.Env = append(c.Env, os.Environ()...)
	// I have no idea if this is right, but it seems to work.
	c.Env = append(c.Env, "TERM=xterm")
	pty, err := pty.Start(c)
	if err != nil {
		panic(err)
	}

	v := vt{vt100.NewVT100(24, 80), new(sync.Mutex)}
	vtexport.Export("/debug/vt100", v.VT100, v)
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.DefaultServeMux,
	}

	vReaderRaw, vWriter := io.Pipe()
	vReader := bufio.NewReader(vReaderRaw)

	dupOut, err := ioutil.TempFile("", "nh_output.txt")
	if err != nil {
		panic(err)
	}

	exit := make(chan struct{}, 0)
	go func() {
		for {
			_, err := io.Copy(pty, os.Stdin)
			if err != nil {
				if err != io.EOF {
					glog.Error(err)
				}
				return
			}
		}
	}()

	go func() {
		defer func() { exit <- struct{}{} }()
		multi := io.MultiWriter(os.Stdout, dupOut, vWriter)
		for {
			_, err := io.Copy(multi, pty)
			if err != nil {
				if err != io.EOF {
					glog.Error(err)
				}
				return
			}
		}
	}()

	go func() {
		for {
			cmd, err := vt100.Decode(vReader)
			if err == nil {
				v.Lock()
				err = v.Process(cmd)
				v.Unlock()
			}
			if err == nil {
				continue
			}
			if _, isUnsupported := err.(vt100.UnsupportedError); isUnsupported {
				// This gets exported through an expvar.
				continue
			}
			if err != io.EOF {
				glog.Error(err)
			}
			return
		}
	}()

	downServer, err := httpdown.HTTP{
		StopTimeout: time.Second * 5,
	}.ListenAndServe(&server)
	if err != nil {
		panic(err)
	}

	<-exit
	downServer.Stop()
	downServer.Wait()
}
