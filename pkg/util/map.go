package util

// MapErr is similar to lo.Map, but handles error in iteratee function
func MapErr[T any, R any](collection []T, iteratee func(T, int) (R, error)) ([]R, error) {
	result := make([]R, len(collection))

	for i, item := range collection {
		res, err := iteratee(item, i)
		if err != nil {
			return nil, err
		}
		result[i] = res
	}

	return result, nil
}
