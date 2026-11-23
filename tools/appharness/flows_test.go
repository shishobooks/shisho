package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const flowsDir = "../../mobile/harness/flows"

// flowCommands returns the commands of the Maestro flow at path, after its
// header, and of every nested command list in them, in order.
func flowCommands(t *testing.T, path string) []*yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // The repository's own flows.
	require.NoError(t, err)
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		docs = append(docs, &doc)
	}
	require.NotEmpty(t, docs, path)
	var commands []*yaml.Node
	var walk func(list *yaml.Node)
	walk = func(list *yaml.Node) {
		for _, command := range list.Content {
			commands = append(commands, command)
			if command.Kind != yaml.MappingNode {
				continue
			}
			for _, args := range command.Content {
				if args.Kind != yaml.MappingNode {
					continue
				}
				for i := 0; i+1 < len(args.Content); i += 2 {
					if args.Content[i].Value == "commands" {
						walk(args.Content[i+1])
					}
				}
			}
		}
	}
	walk(docs[len(docs)-1].Content[0])
	return commands
}

// commandName returns a command's name, such as "launchApp", and its
// arguments' mapping, or nil.
func commandName(command *yaml.Node) (string, *yaml.Node) {
	if command.Kind == yaml.ScalarNode {
		return command.Value, nil
	}
	if command.Kind == yaml.MappingNode && len(command.Content) == 2 {
		if command.Content[1].Kind == yaml.MappingNode {
			return command.Content[0].Value, command.Content[1]
		}
		return command.Content[0].Value, nil
	}
	return "", nil
}

func argument(args *yaml.Node, key string) *yaml.Node {
	if args == nil {
		return nil
	}
	for i := 0; i+1 < len(args.Content); i += 2 {
		if args.Content[i].Value == key {
			return args.Content[i+1]
		}
	}
	return nil
}

// Every launch skips Maestro's permission grants, which take seconds on CI,
// and one after the flow stopped the app doesn't stop it again.
func TestFlowLaunchesSkipPermissionsAndASecondStop(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob(filepath.Join(flowsDir, "*.yaml"))
	require.NoError(t, err)
	subflows, err := filepath.Glob(filepath.Join(flowsDir, "subflows", "*.yaml"))
	require.NoError(t, err)
	launches := 0
	for _, path := range append(paths, subflows...) {
		if filepath.Base(path) == "config.yaml" {
			continue
		}
		stopped := false
		for _, command := range flowCommands(t, path) {
			name, args := commandName(command)
			switch name {
			case "stopApp", "killApp":
				stopped = true
			case "launchApp":
				launches++
				permissions := argument(args, "permissions")
				assert.True(t, permissions != nil && permissions.Kind == yaml.MappingNode && len(permissions.Content) == 0,
					"%s: launchApp passes permissions: {}", path)
				if stopped {
					stop := argument(args, "stopApp")
					assert.True(t, stop != nil && stop.Value == "false",
						"%s: launchApp after stopApp passes stopApp: false", path)
				}
				stopped = false
			}
		}
	}
	assert.Positive(t, launches)
}

// flowSeconds names only flows that exist: a renamed flow would otherwise
// count as the median and unbalance the shards.
func TestFlowSecondsNameRealFlows(t *testing.T) {
	t.Parallel()
	flows, err := topLevelFlows(flowsDir)
	require.NoError(t, err)
	for name := range flowSeconds {
		assert.Contains(t, flows, name)
	}
}

// The harness finds the flows Maestro would, .yml as well as .yaml.
func TestTopLevelFlowsFindsBothExtensions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a.yaml", "b.yml", "config.yaml", "notes.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("appId: x\n---\n"), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subflows"), 0o755))
	flows, err := topLevelFlows(dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.yaml", "b.yml"}, flows)
}
