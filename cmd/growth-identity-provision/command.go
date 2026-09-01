package main

import (
	"strings"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

const provisionCreateCommand = "create"

const (
	accountIDFlag    = "--account-id"
	loginNameFlag    = "--login-name"
	principalIDFlag  = "--principal-id"
	passwordFileFlag = "--password-file"
)

// provisionCommand contains only reviewed create inputs. Account lifecycle,
// credential versions, timestamps, authorization bindings, and a password
// envelope are deliberately not caller-selectable.
type provisionCommand struct {
	accountID    identity.AccountID
	loginName    identity.LoginName
	principalID  identity.PrincipalID
	passwordFile string
}

// parseProvisionCommand implements one deliberately small grammar:
//
//	create --account-id VALUE --login-name VALUE
//	       --principal-id VALUE --password-file PATH
//
// Flags may be reordered, but every flag must occur exactly once and the
// separate value form is mandatory. In particular, the command has no inline
// password, precomputed envelope, role, status, update, delete, or upsert path.
func parseProvisionCommand(args []string) (provisionCommand, bool) {
	if len(args) != 9 || args[0] != provisionCreateCommand {
		return provisionCommand{}, false
	}

	values := make(map[string]string, 4)
	for index := 1; index < len(args); index += 2 {
		flagName := args[index]
		if _, duplicate := values[flagName]; duplicate {
			return provisionCommand{}, false
		}
		switch flagName {
		case accountIDFlag, loginNameFlag, principalIDFlag, passwordFileFlag:
		default:
			return provisionCommand{}, false
		}
		values[flagName] = args[index+1]
	}
	if len(values) != 4 {
		return provisionCommand{}, false
	}

	accountID, err := identity.NewAccountID(values[accountIDFlag])
	if err != nil {
		return provisionCommand{}, false
	}
	loginName, err := identity.NewLoginName(values[loginNameFlag])
	if err != nil {
		return provisionCommand{}, false
	}
	principalID, err := identity.NewPrincipalID(values[principalIDFlag])
	if err != nil {
		return provisionCommand{}, false
	}
	passwordFile := values[passwordFileFlag]
	if passwordFile == "" || strings.IndexByte(passwordFile, 0) >= 0 {
		return provisionCommand{}, false
	}

	return provisionCommand{
		accountID:    accountID,
		loginName:    loginName,
		principalID:  principalID,
		passwordFile: passwordFile,
	}, true
}
