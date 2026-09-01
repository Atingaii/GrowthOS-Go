package main

import (
	"reflect"
	"testing"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestParseProvisionCommandAcceptsExactCreateGrammarInAnyFlagOrder(t *testing.T) {
	want := provisionCommand{
		accountID:    identity.AccountID("account.alex"),
		loginName:    identity.LoginName("alex.rivera"),
		principalID:  identity.PrincipalID("principal.alex"),
		passwordFile: "/run/secrets/alex-password",
	}
	tests := [][]string{
		{
			"create",
			"--account-id", "account.alex",
			"--login-name", "alex.rivera",
			"--principal-id", "principal.alex",
			"--password-file", "/run/secrets/alex-password",
		},
		{
			"create",
			"--password-file", "/run/secrets/alex-password",
			"--principal-id", "principal.alex",
			"--account-id", "account.alex",
			"--login-name", "alex.rivera",
		},
	}

	for _, args := range tests {
		got, ok := parseProvisionCommand(args)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("parseProvisionCommand() = %#v, %t, want exact typed create command", got, ok)
		}
	}
}

func TestParseProvisionCommandRejectsEveryUnreviewedOperationAndInput(t *testing.T) {
	valid := []string{
		"create",
		"--account-id", "account.alex",
		"--login-name", "alex.rivera",
		"--principal-id", "principal.alex",
		"--password-file", "/run/secrets/alex-password",
	}
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "command only", args: []string{"create"}},
		{name: "uppercase command", args: replaceProvisionArg(valid, 0, "CREATE")},
		{name: "upsert", args: replaceProvisionArg(valid, 0, "upsert")},
		{name: "update", args: replaceProvisionArg(valid, 0, "update")},
		{name: "delete", args: replaceProvisionArg(valid, 0, "delete")},
		{name: "raw password", args: replaceProvisionArg(valid, 7, "--password")},
		{name: "precomputed envelope", args: replaceProvisionArg(valid, 7, "--password-envelope")},
		{name: "role", args: append(append([]string{}, valid...), "--role", "admin")},
		{name: "status", args: replaceProvisionArg(valid, 7, "--status")},
		{name: "credential version", args: replaceProvisionArg(valid, 7, "--credential-version")},
		{name: "unknown flag", args: replaceProvisionArg(valid, 1, "--account")},
		{name: "equals form", args: replaceProvisionArg(valid, 1, "--account-id=account.alex")},
		{name: "missing value", args: valid[:8]},
		{name: "extra positional", args: append(append([]string{}, valid...), "extra")},
		{name: "duplicate", args: []string{
			"create",
			"--account-id", "account.alex",
			"--account-id", "account.other",
			"--principal-id", "principal.alex",
			"--password-file", "/run/secrets/alex-password",
		}},
		{name: "missing required through replacement", args: replaceProvisionArg(valid, 1, "--login-name")},
		{name: "invalid account", args: replaceProvisionArg(valid, 2, "Account.Alex")},
		{name: "invalid login", args: replaceProvisionArg(valid, 4, "Alex Rivera")},
		{name: "invalid principal", args: replaceProvisionArg(valid, 6, "principal/")},
		{name: "empty password file", args: replaceProvisionArg(valid, 8, "")},
		{name: "nul password file", args: replaceProvisionArg(valid, 8, "private\x00path")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := parseProvisionCommand(test.args); ok || got != (provisionCommand{}) {
				t.Fatalf("parseProvisionCommand(unreviewed input) = %#v, %t", got, ok)
			}
		})
	}
}

func replaceProvisionArg(source []string, index int, value string) []string {
	result := append([]string(nil), source...)
	result[index] = value
	return result
}
