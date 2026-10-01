package ledger

import (
	"fmt"
	"strconv"
	"strings"
)

// maxKeyLen is stock_movements.idempotency_key VARCHAR(128).
const maxKeyLen = 128

// IdempotencyKey builds {source}:{ref_type}:{ref_id}:{type}:{barcode}.
// The same command (same source document, type and barcode) always yields
// the same key, so a retry writes nothing new. Callers that move one barcode
// twice for one document (two cuts from one roll in one service) use a
// finer reference (the service line, not the service).
func IdempotencyKey(source, refType string, refID int64, typ MovementType, barcode string) (string, error) {
	parts := []string{source, refType, strconv.FormatInt(refID, 10), string(typ), barcode}
	for i, p := range parts {
		if strings.TrimSpace(p) == "" || (i < 2 && strings.Contains(p, ":")) {
			return "", fmt.Errorf("%w: idempotency key part %d %q", ErrInvalidMovement, i, p)
		}
	}
	if refID <= 0 {
		return "", fmt.Errorf("%w: reference id", ErrInvalidMovement)
	}
	key := strings.Join(parts, ":")
	if len(key) > maxKeyLen {
		return "", fmt.Errorf("%w: idempotency key longer than %d", ErrInvalidMovement, maxKeyLen)
	}
	return key, nil
}
