package commerce

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// payloadHash is the idempotency fingerprint: email + canonical lines.
func payloadHash(in CheckoutInput) string {
	lines := append([]LineInput(nil), in.Lines...)
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].VariantID == lines[j].VariantID {
			return lines[i].Quantity < lines[j].Quantity
		}
		return lines[i].VariantID < lines[j].VariantID
	})
	var b strings.Builder
	b.WriteString(strings.TrimSpace(strings.ToLower(in.Email)))
	b.WriteByte('\n')
	for _, l := range lines {
		fmt.Fprintf(&b, "%s:%d\n", l.VariantID, l.Quantity)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// advisoryKey maps an idempotency key onto pg_advisory_xact_lock(k1, k2).
func advisoryKey(key string) (int32, int32) {
	sum := sha256.Sum256([]byte(key))
	k1 := int32(binary.BigEndian.Uint32(sum[0:4]))
	k2 := int32(binary.BigEndian.Uint32(sum[4:8]))
	return k1, k2
}
