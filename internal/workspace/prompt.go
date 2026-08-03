package workspace

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var promptVariable = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)

func (m Manager) RenderPrompt(workspaceID string, values map[string]string, now time.Time) (string, error) {
	item, err := m.Get(workspaceID)
	if err != nil {
		return "", err
	}
	if item.Type != TypeWorkspace {
		return "", fmt.Errorf("item %q is not a workspace", workspaceID)
	}
	vars := cloneMap(item.Vars)
	if vars == nil {
		vars = map[string]string{}
	}
	for key, value := range values {
		vars[key] = value
	}
	vars["workspace.id"] = item.ID
	vars["workspace.name"] = item.Name
	vars["workspace.root"] = item.Root
	vars["date"] = now.Format("2006-01-02")
	vars["time"] = now.Format("15:04:05")
	vars["datetime"] = now.Format(time.RFC3339)
	var unresolved []string
	result := promptVariable.ReplaceAllStringFunc(item.Prompt, func(match string) string {
		key := promptVariable.FindStringSubmatch(match)[1]
		value, ok := vars[key]
		if !ok {
			unresolved = append(unresolved, key)
			return match
		}
		return value
	})
	if len(unresolved) > 0 {
		return "", fmt.Errorf("unresolved prompt variables: %s", strings.Join(uniqueStrings(unresolved), ", "))
	}
	return result, nil
}
