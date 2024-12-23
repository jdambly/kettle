package utils

import "k8s.io/apimachinery/pkg/types"

// NamespacedNameToString converts a NamespacedName to a string
func NamespacedNameToString(name types.NamespacedName) string {
	return name.Namespace + "/" + name.Name
}
