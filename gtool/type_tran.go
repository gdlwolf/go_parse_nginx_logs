package gtool

//[]string转[]interface{}
func Ss2Is(s []string) []interface{} {
	newS := make([]interface{}, len(s))
	for i, v := range s {
		newS[i] = v
	}
	return newS
}
