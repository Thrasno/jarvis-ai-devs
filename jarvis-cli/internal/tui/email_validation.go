package tui

import (
	"errors"
	"strings"
)

const invalidHiveCloudEmailMessage = "Email inválido. Ingresa un email válido, por ejemplo usuario@dominio.com."

func normalizeHiveCloudEmail(input string) (string, error) {
	email := strings.TrimSpace(input)
	if email == "" {
		return "", nil
	}
	if strings.ContainsAny(email, " \t\r\n") {
		return "", errors.New(invalidHiveCloudEmailMessage)
	}
	if strings.Count(email, "@") != 1 {
		return "", errors.New(invalidHiveCloudEmailMessage)
	}
	local, domain, _ := strings.Cut(email, "@")
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return "", errors.New(invalidHiveCloudEmailMessage)
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", errors.New(invalidHiveCloudEmailMessage)
	}
	return email, nil
}
