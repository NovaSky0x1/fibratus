//go:build !windows

package tamper

import "fmt"

// EnrollmentData holds all enrollment fields.
type EnrollmentData struct {
	ServerURL string
	OrgID     string
	AgentID   string
	AgentCert []byte
	AgentKey  []byte
	CACert    []byte
}

func StoreEnrollment(_ *EnrollmentData) error         { return fmt.Errorf("not supported") }
func LoadEnrollment() *EnrollmentData                  { return nil }
func StoreAgentID(_ string) error                      { return fmt.Errorf("not supported") }
func MigrateFilesToRegistry(_ string) bool              { return false }
func StoreState(_, _ string) error                      { return nil }
func LoadState(_ string) string                         { return "" }
func StoreEncryptedState(_ string, _ []byte) error      { return nil }
func LoadEncryptedState(_ string) []byte                { return nil }
func ProtectRegistryKeys()                              {}
