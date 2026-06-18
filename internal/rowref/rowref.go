package rowref

import (
	"fmt"
	"strconv"
	"strings"
)

func Encode(row int, candidateID string) string {
	return fmt.Sprintf("%d|%s", row, candidateID)
}

func Decode(s string) (row int, candidateID string, err error) {
	parts := strings.SplitN(s, "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, "", fmt.Errorf("invalid row reference %q", s)
	}
	row, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid row reference %q: %w", s, err)
	}
	return row, parts[1], nil
}

func CustomID(action string, row int, candidateID string) string {
	return action + "|" + Encode(row, candidateID)
}

func DecodeCustomID(customID string) (action string, row int, candidateID string, err error) {
	idx := strings.Index(customID, "|")
	if idx < 0 {
		return "", 0, "", fmt.Errorf("invalid custom id %q", customID)
	}
	action = customID[:idx]
	row, candidateID, err = Decode(customID[idx+1:])
	return action, row, candidateID, err
}
