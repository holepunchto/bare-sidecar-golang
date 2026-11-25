// Package sidecar starts and manages Bare sidecar processes.
package sidecar

import (
	"io"
	"os/exec"
	"sync"
)

// Read by the embedder, which opens `Bare.IPC` from the descriptor, or on
// Windows the handle, it names.
const ipcEnv = "BARE_SIDECAR_IPC"

// A Sidecar is a Bare process connected to its `Bare.IPC`.
//
// Reading yields the data that the process writes to `Bare.IPC` until it ends
// the connection and exits, and writing sends data to it. One goroutine may
// read while another writes. Closing the sidecar closes the connection, then
// terminates the process and waits for it.
type Sidecar struct {
	// Cmd is the command running the process. It must be waited for with
	// Wait rather than Cmd.Wait.
	Cmd *exec.Cmd

	ipc  *ipc
	once sync.Once
	err  error
}

// Start starts cmd with an IPC connection to it. The command must run a Bare
// executable built with the embedder of this module, such as one downloaded
// with the prebuild package, with the entry point as its first argument and
// the arguments that follow it as the rest of `Bare.argv`.
func Start(cmd *exec.Cmd) (*Sidecar, error) {
	ipc, remote, err := pair()
	if err != nil {
		return nil, err
	}

	defer remote.close()

	cmd.Env = append(cmd.Environ(), ipcEnv+"="+remote.pass(cmd))

	if err := cmd.Start(); err != nil {
		ipc.close()

		return nil, err
	}

	return &Sidecar{Cmd: cmd, ipc: ipc}, nil
}

// Read reads data that the process has written to `Bare.IPC`, returning
// io.EOF once it has ended the connection and has been waited for.
func (s *Sidecar) Read(b []byte) (int, error) {
	n, err := s.ipc.read(b)

	if err == io.EOF && len(b) > 0 {
		s.Wait()
	}

	return n, err
}

// Write sends data to `Bare.IPC` of the process.
func (s *Sidecar) Write(b []byte) (int, error) {
	return s.ipc.write(b)
}

// Close closes the connection, then terminates the process, as Terminate
// does, and waits for it.
func (s *Sidecar) Close() error {
	err := s.ipc.close()

	terminate(s.Cmd.Process)
	s.Wait()

	return err
}

// Wait waits for the process to exit, as Cmd.Wait does, and returns the same
// error to every caller. Reading to io.EOF has already waited for it.
func (s *Sidecar) Wait() error {
	s.once.Do(func() {
		s.err = s.Cmd.Wait()
	})

	return s.err
}

// Terminate asks the process to exit, with `SIGTERM` or on Windows by
// terminating it, unless it has already exited.
func (s *Sidecar) Terminate() error {
	return terminate(s.Cmd.Process)
}
