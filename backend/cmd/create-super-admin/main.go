// Command create-super-admin bootstraps a platform super_admin user.
//
// Usage (repo root):
//
//	make create-super-admin SA_EMAIL=admin@example.com SA_PASSWORD='Password1'
//
// Or from backend/:
//
//	make create-super-admin SA_EMAIL=admin@example.com SA_PASSWORD='Password1'
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	authrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
)

func main() {
	email := flag.String("email", "admin@example.com", "super admin email")
	pass := flag.String("password", "Password1", "super admin password (min 8, upper+lower+digit)")
	name := flag.String("name", "Platform", "first name")
	surname := flag.String("surname", "Admin", "last name")
	flag.Parse()

	*email = strings.ToLower(strings.TrimSpace(*email))
	*name = strings.TrimSpace(*name)
	*surname = strings.TrimSpace(*surname)
	if *email == "" || *name == "" || *surname == "" {
		fatal("email, name, and surname are required")
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	hash, err := password.Hash(*pass)
	if err != nil {
		fatal("password: %v", err)
	}

	repo := authrepo.NewPostgres(pool, database.NewQueries(pool))
	user, created, err := repo.UpsertSuperAdmin(ctx, *email, *name, *surname, hash)
	if err != nil {
		fatal("%v", err)
	}

	action := "updated"
	if created {
		action = "created"
	}
	fmt.Printf("super_admin %s successfully\n", action)
	fmt.Printf("  uuid:   %s\n", user.UUID)
	fmt.Printf("  email:  %s\n", user.Email)
	fmt.Printf("  name:   %s %s\n", user.Name, user.Surname)
	fmt.Printf("  status: %s\n", user.Status)
	fmt.Printf("  role:   super_admin\n")
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "create-super-admin: "+format+"\n", args...)
	os.Exit(1)
}
