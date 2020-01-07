package tcglog

import (
	"bytes"
	"fmt"
	"strconv"
)

func makeDefaultFormatter(s fmt.State, f rune) string {
	var builder bytes.Buffer
	builder.WriteString("%%")
	for _, flag := range [...]int{'+', '-', '#', ' ', '0'} {
		if s.Flag(flag) {
			fmt.Fprintf(&builder, "%c", flag)
		}
	}
	if width, ok := s.Width(); ok {
		fmt.Fprintf(&builder, "%d", width)
	}
	if prec, ok := s.Precision(); ok {
		fmt.Fprintf(&builder, ".%d", prec)
	}
	fmt.Fprintf(&builder, "%c", f)
	return builder.String()
}

type PCRArgList []PCRIndex

func (l *PCRArgList) String() string {
	var builder bytes.Buffer
	for i, pcr := range *l {
		if i > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, "%d", pcr)
	}
	return builder.String()
}

func (l *PCRArgList) Set(value string) error {
	v, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return err
	}
	*l = append(*l, PCRIndex(v))
	return nil
}

func ParseAlgorithm(alg string) (AlgorithmId, error) {
	switch alg {
	case "sha1":
		return AlgorithmSha1, nil
	case "sha256":
		return AlgorithmSha256, nil
	case "sha384":
		return AlgorithmSha384, nil
	case "sha512":
		return AlgorithmSha512, nil
	default:
		return 0, fmt.Errorf("Unrecognized algorithm \"%s\"", alg)
	}
}
