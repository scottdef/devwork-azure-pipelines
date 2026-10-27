package dynatrace

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// Stats counts what a Shipper did.
type Stats struct {
	Requests int64 `json:"requests"`
	Records  int64 `json:"records"`  // records in accepted or partial requests
	Bytes    int64 `json:"bytes"`    // uncompressed payload bytes sent
	Partial  int64 `json:"partial"`  // requests answered 200 (partial success)
	Stale    int64 `json:"stale"`    // records refused as out of time range
	Rejected int64 `json:"rejected"` // single records refused even alone (413/400)
	Splits   int64 `json:"splits"`   // 413-driven batch splits
	Retries  int64 `json:"retries"`
}

// Shipper accumulates encoded records and posts them in batches no
// larger than MaxBytes or MaxRecords. Records are flushed strictly in
// the order added; Added and Flushed are sequence numbers the caller
// can use to know which of its units of work are durably shipped.
type Shipper struct {
	Poster     Poster
	MaxBytes   int
	MaxRecords int
	DryRun     io.Writer // if set, write NDJSON here instead of posting
	Log        *slog.Logger

	recs   [][]byte
	size   int
	header string
	added  int64
	flushd int64
	Stats  Stats
}

// Added is the number of records handed to Add.
func (s *Shipper) Added() int64 { return s.added }

// Flushed is the number of records whose request has completed (sent,
// or deliberately dropped as stale/rejected).
func (s *Shipper) Flushed() int64 { return s.flushd }

// SetHeader changes the X-Dynatrace-Attr value for subsequent records,
// flushing first if records with a different header are buffered.
func (s *Shipper) SetHeader(ctx context.Context, h string) error {
	if h == s.header {
		return nil
	}
	if err := s.Flush(ctx); err != nil {
		return err
	}
	s.header = h
	return nil
}

// Add buffers one encoded JSON object, flushing when a limit would be
// crossed. A single record larger than MaxBytes is sent on its own.
func (s *Shipper) Add(ctx context.Context, rec []byte) error {
	if len(s.recs) > 0 && (s.size+len(rec)+1 > s.MaxBytes-2 || len(s.recs) >= s.MaxRecords) {
		if err := s.Flush(ctx); err != nil {
			return err
		}
	}
	s.recs = append(s.recs, rec)
	s.size += len(rec) + 1
	s.added++
	return nil
}

// Flush sends everything buffered.
func (s *Shipper) Flush(ctx context.Context) error {
	if len(s.recs) == 0 {
		return nil
	}
	recs := s.recs
	s.recs, s.size = nil, 0
	if err := s.send(ctx, recs); err != nil {
		return err
	}
	s.flushd += int64(len(recs))
	return nil
}

func (s *Shipper) send(ctx context.Context, recs [][]byte) error {
	if s.DryRun != nil {
		for _, r := range recs {
			if _, err := s.DryRun.Write(append(r, '\n')); err != nil {
				return err
			}
		}
		s.Stats.Records += int64(len(recs))
		return nil
	}
	payload := encodeArray(recs)
	resp, err := s.Poster.Post(ctx, payload, s.header)
	s.Stats.Retries += int64(resp.Retries)
	switch {
	case errors.Is(err, ErrTooLarge) && len(recs) > 1:
		s.Stats.Splits++
		half := len(recs) / 2
		if err := s.send(ctx, recs[:half]); err != nil {
			return err
		}
		return s.send(ctx, recs[half:])
	case errors.Is(err, ErrTooLarge):
		s.Stats.Rejected++
		s.log().Warn("dropping a record Dynatrace refuses even alone", "bytes", len(recs[0]))
		return nil
	case err != nil:
		var re *RejectedError
		if errors.As(err, &re) && len(recs) > 1 && re.Code == 400 {
			// One bad record poisons the request: bisect to find it.
			s.Stats.Splits++
			half := len(recs) / 2
			if err := s.send(ctx, recs[:half]); err != nil {
				return err
			}
			return s.send(ctx, recs[half:])
		}
		if errors.As(err, &re) && len(recs) == 1 {
			s.Stats.Rejected++
			s.log().Warn("record rejected", "status", re.Code, "message", re.Message)
			return nil
		}
		return err
	}
	s.Stats.Requests++
	s.Stats.Bytes += int64(len(payload))
	switch resp.Outcome {
	case Accepted:
		s.Stats.Records += int64(len(recs))
	case Partial:
		s.Stats.Records += int64(len(recs))
		s.Stats.Partial++
		s.log().Warn("partial success", "records", len(recs), "message", clip(resp.Message))
	case Stale:
		s.Stats.Stale += int64(len(recs))
		s.log().Warn("all records out of time range", "records", len(recs))
	}
	return nil
}

func (s *Shipper) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return s.Log
}

func encodeArray(recs [][]byte) []byte {
	n := 2
	for _, r := range recs {
		n += len(r) + 1
	}
	b := make([]byte, 0, n)
	b = append(b, '[')
	for i, r := range recs {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, r...)
	}
	return append(b, ']')
}
