package sidecar

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const bufferSize = 65536

var counter atomic.Uint64

// The server end of a named pipe, which stays open until closed, so that
// disconnecting the process from it never invalidates a handle in use.
type ipc struct {
	file *os.File
	pipe windows.Handle
}

// The client end of the named pipe, which is handed to the process.
type remote struct {
	handle windows.Handle
}

func pair() (*ipc, *remote, error) {
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(
		`\\.\pipe\bare-sidecar-%d-%d-%d`,
		os.Getpid(),
		counter.Add(1),
		time.Now().UnixNano(),
	))
	if err != nil {
		return nil, nil, err
	}

	// A single instance that this process connects to before the child
	// exists, so that no other process can connect in its place.
	pipe, err := windows.CreateNamedPipe(
		name,
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1,
		bufferSize,
		bufferSize,
		0,
		nil,
	)
	if err != nil {
		return nil, nil, os.NewSyscallError("CreateNamedPipe", err)
	}

	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		windows.CloseHandle(pipe)

		return nil, nil, os.NewSyscallError("CreateFile", err)
	}

	err = windows.ConnectNamedPipe(pipe, &windows.Overlapped{})
	if err != nil && err != windows.ERROR_PIPE_CONNECTED {
		windows.CloseHandle(handle)
		windows.CloseHandle(pipe)

		return nil, nil, os.NewSyscallError("ConnectNamedPipe", err)
	}

	// Opened for overlapped I/O, so that reads and writes don't wait for each
	// other and the file is pollable by the runtime.
	file := os.NewFile(uintptr(pipe), "ipc")

	return &ipc{file, pipe}, &remote{handle}, nil
}

// Inheritable only by the process, as the runtime passes the handles that a
// process inherits explicitly.
func (r *remote) pass(cmd *exec.Cmd) string {
	windows.SetHandleInformation(r.handle, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.AdditionalInheritedHandles = append(slices.Clip(cmd.SysProcAttr.AdditionalInheritedHandles), syscall.Handle(r.handle))

	return strconv.FormatUint(uint64(r.handle), 10)
}

func (r *remote) close() {
	windows.CloseHandle(r.handle)
}

func (c *ipc) read(b []byte) (int, error) {
	n, err := c.file.Read(b)

	if isDisconnected(err) {
		err = io.EOF
	}

	// Named pipes cannot be half closed, so the process only sees the
	// connection end once it is disconnected.
	if err == io.EOF && len(b) > 0 {
		windows.DisconnectNamedPipe(c.pipe)
	}

	return n, err
}

func (c *ipc) write(b []byte) (int, error) {
	return c.file.Write(b)
}

func (c *ipc) close() error {
	return c.file.Close()
}

func isDisconnected(err error) bool {
	return errors.Is(err, windows.ERROR_BROKEN_PIPE) ||
		errors.Is(err, windows.ERROR_NO_DATA) ||
		errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED)
}

// Windows has no signals, so the process is terminated with exit code 1.
func terminate(process *os.Process) error {
	err := process.Kill()

	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return err
}
