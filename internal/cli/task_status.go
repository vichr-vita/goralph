package cli

import "goralph/internal/db"

func validateTaskStatus(status string) error {
	return db.ValidateTaskStatus(status)
}
