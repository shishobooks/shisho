package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/dop251/goja"
)

// defaultShellTimeout is the default timeout for shell command execution.
var defaultShellTimeout = 5 * time.Minute

// injectShellNamespace sets up shisho.shell with the exec method.
// The exec function executes allowed commands as subprocesses:
// - Requires shellAccess capability with command in allowlist
// - Uses exec.Command directly (no shell) to prevent injection
// - Respects context cancellation for timeout management.
func injectShellNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	shellObj := vm.NewObject()
	if err := shishoObj.Set("shell", shellObj); err != nil {
		return fmt.Errorf("failed to set shisho.shell: %w", err)
	}

	shellObj.Set("exec", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		// Check capability
		if rt.manifest.Capabilities.ShellAccess == nil {
			panic(vm.ToValue("shisho.shell.exec: plugin does not declare shellAccess capability"))
		}

		// Get command argument
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.shell.exec: command and args arguments are required"))
		}

		command := call.Argument(0).String()

		// Validate command against allowlist
		allowed := false
		for _, cmd := range rt.manifest.Capabilities.ShellAccess.Commands {
			if cmd == command {
				allowed = true
				break
			}
		}
		if !allowed {
			panic(vm.ToValue(fmt.Sprintf("shisho.shell.exec: command %q is not in allowed list", command)))
		}

		// Get args array
		argsVal := call.Argument(1)
		argsObj := argsVal.ToObject(vm)
		length := int(argsObj.Get("length").ToInteger())

		args := make([]string, 0, length)
		for i := 0; i < length; i++ {
			args = append(args, argsObj.Get(strconv.Itoa(i)).String())
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), defaultShellTimeout)
		defer cancel()

		// Build command - use exec.Command directly (no shell)
		cmd := exec.CommandContext(ctx, command, args...)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Run the command
		err := cmd.Run()

		// Determine exit code
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				// Command failed to start or was killed
				panic(vm.ToValue(fmt.Sprintf("shisho.shell.exec: failed to execute: %v", err)))
			}
		}

		// Build result object
		result := vm.NewObject()
		result.Set("exitCode", exitCode)      //nolint:errcheck
		result.Set("stdout", stdout.String()) //nolint:errcheck
		result.Set("stderr", stderr.String()) //nolint:errcheck

		return result
	})

	return nil
}
