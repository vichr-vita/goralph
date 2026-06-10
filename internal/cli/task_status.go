package cli

import "github.com/vichr-vita/goralph/internal/db"

func validateTaskStatus(status string) error {
	return db.ValidateTaskStatus(status)
}
