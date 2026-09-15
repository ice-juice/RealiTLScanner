//go:build darwin

package desktop

import "syscall"

func raiseFileLimit() {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return
	}
	const want = 8192
	if lim.Cur >= want {
		return
	}
	lim.Cur = want
	if lim.Max > 0 && lim.Cur > lim.Max {
		lim.Cur = lim.Max
	}
	_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim)
}
