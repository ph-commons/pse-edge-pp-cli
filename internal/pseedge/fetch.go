// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pseedge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// HistoryBaseURL is the DisclosureCht.ax endpoint (JSON POST, no auth).
const HistoryBaseURL = "https://edge.pse.com.ph/common/DisclosureCht.ax"

// ReadPoster is the slice of the generated internal/client.Client this
// package needs: a read-only POST that survives verify-mode (the endpoint
// rides a mutating verb on the wire but never mutates remote state).
type ReadPoster interface {
	PostQueryWithParams(ctx context.Context, path string, params map[string]string, body any) (json.RawMessage, int, error)
}

// historyRequest is the DisclosureCht.ax JSON body. Dates are MM-DD-YYYY
// per the observed contract.
type historyRequest struct {
	CmpyID     int    `json:"cmpy_id"`
	SecurityID int    `json:"security_id"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
}

// FetchHistory POSTs common/DisclosureCht.ax for one security and returns
// completed daily bars in [start, end]. Rows come back only for sessions
// that actually traded (weekends/holidays are naturally absent).
func FetchHistory(ctx context.Context, c ReadPoster, cmpyID, securityID int, start, end time.Time) ([]EODRow, error) {
	body := historyRequest{
		CmpyID:     cmpyID,
		SecurityID: securityID,
		StartDate:  start.Format("01-02-2006"),
		EndDate:    end.Format("01-02-2006"),
	}
	data, status, err := c.PostQueryWithParams(ctx, HistoryBaseURL, nil, body)
	if err != nil {
		return nil, fmt.Errorf("pse-edge common/DisclosureCht.ax cmpy_id=%d security_id=%d: %w", cmpyID, securityID, err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("pse-edge common/DisclosureCht.ax cmpy_id=%d security_id=%d: HTTP %d", cmpyID, securityID, status)
	}
	return ParseHistoryResponse(data)
}
