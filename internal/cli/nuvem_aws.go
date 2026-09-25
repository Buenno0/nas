//go:build !nocloud

package cli

// O adapter S3 se registra no init. Com -tags nocloud este import some e o
// binário sai sem o SDK da AWS.
import _ "nas/internal/cloud/aws"
