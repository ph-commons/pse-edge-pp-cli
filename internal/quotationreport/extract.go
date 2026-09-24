// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// ExtractLayout runs Poppler pdftotext -layout. A missing binary is its own
// status so a failed extract is not reported as an empty quotation table.
func ExtractLayout(ctx context.Context, pdfPath string) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", statusErr(StatusTextExtractorMissing, "PDF text extraction requires pdftotext; install Poppler")
	}
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", pdfPath, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
			return "", statusErr(StatusAcquisitionFailed, "pdftotext canceled")
		}
		detail := stderr.String()
		if detail == "" {
			detail = err.Error()
		}
		return "", statusErr(StatusMalformedDocument, fmt.Sprintf("pdftotext failed: %s", detail))
	}
	if stdout.Len() == 0 {
		return "", statusErr(StatusMalformedDocument, "PDF had no text layer")
	}
	return stdout.String(), nil
}
