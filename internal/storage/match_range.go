package storage

import "time"

// MinMatchDate — нижняя граница отключена; в анализе участвуют все матчи из БД.
var MinMatchDate = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func minMatchDateRFC() string {
	return MinMatchDate.UTC().Format(time.RFC3339)
}
