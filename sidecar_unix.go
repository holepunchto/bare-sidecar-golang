//go:build unix

package sidecar

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"syscall"
)

type ipc struct {
	conn *net.UnixConn
}

// The end of the connection that is handed to the process.
type remote struct {
	file *os.File
}

func pair() (*ipc, *remote, error) {
	// Held so that no process spawned in the meantime inherits the
	// descriptors before they are marked close-on-exec.
	syscall.ForkLock.RLock()

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fds[0])
		syscall.CloseOnExec(fds[1])
	}

	syscall.ForkLock.RUnlock()

	if err != nil {
		return nil, nil, os.NewSyscallError("socketpair", err)
	}

	local := os.NewFile(uintptr(fds[0]), "ipc")
	defer local.Close()

	remote := &remote{os.NewFile(uintptr(fds[1]), "ipc")}

	conn, err := net.FileConn(local)
	if err != nil {
		remote.close()

		return nil, nil, err
	}

	return &ipc{conn.(*net.UnixConn)}, remote, nil
}

// Passed as the descriptor that follows those of cmd.ExtraFiles in the
// process.
func (r *remote) pass(cmd *exec.Cmd) string {
	fd := 3 + len(cmd.ExtraFiles)

	cmd.ExtraFiles = append(slices.Clip(cmd.ExtraFiles), r.file)

	return strconv.Itoa(fd)
}

func (r *remote) close() {
	r.file.Close()
}

func (c *ipc) read(b []byte) (int, error) {
	n, err := c.conn.Read(b)

	// On Linux, a process that exits with data still unread resets the
	// connection rather than ending it.
	if errors.Is(err, syscall.ECONNRESET) {
		err = io.EOF
	}

	// The process exits once the connection has closed, which takes both ends
	// to have ended.
	if err == io.EOF && len(b) > 0 {
		c.conn.CloseWrite()
	}

	return n, err
}

func (c *ipc) write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (c *ipc) close() error {
	return c.conn.Close()
}

func terminate(process *os.Process) error {
	err := process.Signal(syscall.SIGTERM)

	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return err
}
