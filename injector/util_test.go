package injector

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestGetFieldByNestedName_ValidNestedName(t *testing.T) {
	type TestStruct struct {
		Field1 struct {
			Field2 int
		}
	}

	v := reflect.ValueOf(&TestStruct{})
	nestedName := "Field1.Field2"

	result := GetFieldByNestedName(v, nestedName)

	assert.True(t, result.IsValid())
	assert.Equal(t, reflect.Int, result.Kind())
}

func TestGetFieldByNestedName_InvalidNestedName(t *testing.T) {
	type TestStruct struct {
		Field1 struct {
			Field2 int
		}
	}

	v := reflect.ValueOf(&TestStruct{})
	nestedName := "Field1.NonExistentField"

	result := GetFieldByNestedName(v, nestedName)

	assert.False(t, result.IsValid())
}

func TestGetFieldByNestedName_EmptyNestedName(t *testing.T) {
	type TestStruct struct {
		Field1 struct {
			Field2 int
		}
	}

	v := reflect.ValueOf(&TestStruct{})
	nestedName := ""

	result := GetFieldByNestedName(v, nestedName)

	assert.True(t, result.IsValid())
	assert.Equal(t, reflect.Ptr, result.Kind())
}

func TestGetFieldByNestedName_NonStructValue(t *testing.T) {
	v := reflect.ValueOf(42)
	nestedName := "Field1"

	result := GetFieldByNestedName(v, nestedName)

	assert.False(t, result.IsValid())
}
