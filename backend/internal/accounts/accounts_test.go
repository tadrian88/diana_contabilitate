package accounts

import "testing"

func TestValidatePostingAccount(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		allowed func(string) bool
		code    ValidationCode
	}{
		{"missing", nil, nil, AccountNotFound},
		{"inactive", &Account{Code: "6281", Active: false, Postable: true}, nil, AccountInactive},
		{"synthetic", &Account{Code: "628", Active: true, Postable: false, Synthetic: true}, nil, AccountNotPostable},
		{"outside profile", &Account{Code: "6281", Active: true, Postable: true}, func(string) bool { return false }, AccountNotAllowedProfile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePostingAccount(test.account, "628", test.allowed)
			issue, ok := AsValidationIssue(err)
			if !ok || issue.Code != test.code {
				t.Fatalf("issue=%#v err=%v", issue, err)
			}
		})
	}
	if err := ValidatePostingAccount(&Account{Code: "6281", Active: true, Postable: true}, "6281", func(string) bool { return true }); err != nil {
		t.Fatalf("valid analytic rejected: %v", err)
	}
}
