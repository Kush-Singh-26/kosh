package models

// CountWords returns the number of words in a byte slice using a direct byte loop
func CountWords(s []byte) int {
	count := 0
	inWord := false
	for _, b := range s {
		isSpace := b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
		if !isSpace {
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}
