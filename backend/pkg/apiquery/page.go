package apiquery

// Page is the standard list payload under envelope data.
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

// NewPage builds a Page with a non-nil items slice.
func NewPage[T any](items []T, total int64, limit, offset int32) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Total: total, Limit: limit, Offset: offset}
}

// Slice applies limit/offset to an in-memory slice and returns the page plus total.
func Slice[T any](all []T, limit, offset int32) (page []T, total int64) {
	total = int64(len(all))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if int64(offset) >= total {
		return []T{}, total
	}
	end := int64(offset) + int64(limit)
	if end > total {
		end = total
	}
	return all[offset:end], total
}

// FromSlice builds a standard Page by slicing an in-memory collection.
func FromSlice[T any](all []T, limit, offset int32) Page[T] {
	page, total := Slice(all, limit, offset)
	return NewPage(page, total, limit, offset)
}
