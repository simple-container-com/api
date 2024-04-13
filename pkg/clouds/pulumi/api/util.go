package api

import (
	"fmt"
	"strings"
)

func ExpandStackReference(parentStack string, organization string, projectName string) string {
	parentStackParts := strings.SplitN(parentStack, "/", 3)
	if len(parentStackParts) == 3 {
		return parentStack
	} else if len(parentStackParts) == 2 {
		return fmt.Sprintf("%s/%s", organization, parentStack)
	} else {
		return fmt.Sprintf("%s/%s/%s", organization, projectName, parentStack)
	}
}

func CollapseStackReference(stackRef string) string {
	stackRefParts := strings.SplitN(stackRef, "/", 3)
	return stackRefParts[len(stackRefParts)-1]
}
