// Code generated by pegomock. DO NOT EDIT.
package matchers

import (
	"reflect"
	"github.com/petergtz/pegomock"
	io "io"
)

func AnyIoWriter() io.Writer {
	pegomock.RegisterMatcher(pegomock.NewAnyMatcher(reflect.TypeOf((*(io.Writer))(nil)).Elem()))
	var nullValue io.Writer
	return nullValue
}

func EqIoWriter(value io.Writer) io.Writer {
	pegomock.RegisterMatcher(&pegomock.EqMatcher{Value: value})
	var nullValue io.Writer
	return nullValue
}
