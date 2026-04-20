package analyzer

func sampleData(data any, sampleSize int) any {
	switch v := data.(type) {
	case map[string]any:
		sampledMap := map[string]any{}
		count := 0
		for key, val := range v {
			if count >= sampleSize {
				break
			}
			sampledMap[key] = sampleData(val, sampleSize)
			count++
		}
		return sampledMap
	case []any:
		sampledSlice := make([]any, 0, sampleSize)
		for i, item := range v {
			if i >= sampleSize {
				break
			}
			sampledSlice = append(sampledSlice, sampleData(item, sampleSize))
		}
		return sampledSlice
	default:
		return v
	}
}
