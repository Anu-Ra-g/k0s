//go:build unix

/*
Copyright 2022 k0s authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	exitCheckInterval = 200 * time.Millisecond
)

// killPid signals SIGTERM to a PID and if it's still running after
// s.TimeoutStop sends SIGKILL.
func (s *Supervisor) killPid(pid int) error {
	// Kill the process pid
	deadlineTicker := time.NewTicker(s.TimeoutStop)
	defer deadlineTicker.Stop()
	checkTicker := time.NewTicker(exitCheckInterval)
	defer checkTicker.Stop()

Loop:
	for {
		select {
		case <-checkTicker.C:
			shouldKill, err := s.shouldKillProcess(pid)
			if err != nil {
				return err
			}
			if !shouldKill {
				return nil
			}

			err = syscall.Kill(pid, syscall.SIGTERM)
			if errors.Is(err, syscall.ESRCH) {
				return nil
			} else if err != nil {
				return fmt.Errorf("failed to send SIGTERM: %w", err)
			}
		case <-deadlineTicker.C:
			break Loop
		}
	}

	shouldKill, err := s.shouldKillProcess(pid)
	if err != nil {
		return err
	}
	if !shouldKill {
		return nil
	}

	err = syscall.Kill(pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}
	return nil
}

// maybeKillPidFile checks kills the process in the pidFile if it's has
// the same binary as the supervisor's and also checks that the env
// `_KOS_MANAGED=yes`. This function does not delete the old pidFile as
// this is done by the caller.
func (s *Supervisor) maybeKillPidFile() error {
	pid, err := os.ReadFile(s.PidFile)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to read pid file %s: %w", s.PidFile, err)
	}

	p, err := strconv.Atoi(strings.TrimSuffix(string(pid), "\n"))
	if err != nil {
		return fmt.Errorf("failed to parse pid file %s: %w", s.PidFile, err)
	}

	if err := s.killPid(p); err != nil {
		return fmt.Errorf("failed to kill process with PID %d: %w", p, err)
	}

	return nil
}

func (s *Supervisor) shouldKillProcess(pid int) (bool, error) {
	// only kill process if it has the expected cmd
	if cmd, err := cmdline(pid); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		return false, err
	} else if len(cmd) > 0 && cmd[0] != s.BinPath {
		return false, nil
	}

	//only kill process if it has the _KOS_MANAGED env set
	if env, err := environ(pid); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		return false, err
	} else if !slices.Contains(env, k0sManaged) {
		return false, nil
	}

	return true, nil
}

func cmdline(pid int) ([]string, error) {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %w", syscall.ESRCH, err)
		}
		return nil, fmt.Errorf("failed to read process cmdline: %w", err)
	}

	return strings.Split(string(cmdline), "\x00"), nil
}

func environ(pid int) ([]string, error) {
	env, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %w", syscall.ESRCH, err)
		}
		return nil, fmt.Errorf("failed to read process environ: %w", err)
	}

	return strings.Split(string(env), "\x00"), nil
}
