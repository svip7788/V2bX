package format

func UserTag(tag string, uuid string) string {
	buf := make([]byte, 0, len(tag)+1+len(uuid))
	buf = append(buf, tag...)
	buf = append(buf, '|')
	buf = append(buf, uuid...)
	return string(buf)
}
