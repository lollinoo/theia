package service

import (
	"fmt"
	"os/exec"
)

func isolatedPostgresUser(*exec.Cmd, string) error {
	return fmt.Errorf("isolated backup verification runs in the Linux backend container")
}
