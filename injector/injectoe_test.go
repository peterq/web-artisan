package injector

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"log"
	"testing"
)

type userInput struct {
	Username1 string
	Username2 string
	Role      string `inject:"enum=agent,miner" json:"role"`

	User  *user `inject:"by=Username1|by=Username2"`
	User2 *user `inject:"by=Username2,Role"`
}

type user struct {
	RoleType int64 `inject:"by=hah"`
	Username string
}

func userByUsername(username string) (*user, error) {
	if username == "fuck" || username == "" {
		return nil, errors.New("'fuck' or empty username is not allowed")
	}
	return &user{
		RoleType: 0,
		Username: username,
	}, nil
}

func userByUsernameAndRole(username string, role string) (*user, error) {
	u := &user{
		RoleType: 1,
		Username: "",
	}
	if role == "miner" {
		u.RoleType = 2
	}
	if username == "peter" {
		u.RoleType = 10
		u.Username = username
	} else if username == "" {
		//
	} else {
		return nil, errors.New("cant find user: " + username)
	}
	return u, nil
}

func TestStruct(t *testing.T) {
	inject := New(&Config{
		TagName:      "inject",
		FieldNameTag: "",
	})
	err := inject.AddResolver(userByUsername)
	if err != nil {
		panic(err)
	}
	err = inject.AddResolver(userByUsernameAndRole)
	if err != nil {
		panic(err)
	}
	inject.CacheForStruct(&userInput{})

	param := &userInput{
		Username1: "peter",
		Username2: "",
		Role:      "miner",
	}
	err = inject.Struct(param)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, param.User) {
		return
	}
}

func TestStructMap(t *testing.T) {
	inject := New(&Config{
		TagName:      "inject",
		FieldNameTag: "",
	})
	err := inject.AddResolver(userByUsername)
	if err != nil {
		panic(err)
	}
	err = inject.AddResolver(userByUsernameAndRole)
	if err != nil {
		panic(err)
	}
	inject.CacheForStruct(&userInput{})
	type temp struct {
		Mp map[string]*userInput `inject:"dive"`
	}
	inject.CacheForStruct(&temp{})
	param := temp{
		Mp: map[string]*userInput{
			"a": {
				Username1: "peter",
				Username2: "",
				Role:      "miner",
			},
		},
	}
	err = inject.Struct(&param)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, param.Mp["a"].User) {
		return
	}
	log.Println(param.Mp["a"].User)
}

func TestInject_StructWithCtxData(t *testing.T) {
	inject := New(&Config{
		TagName:      "inject",
		FieldNameTag: "",
	})
	type input struct {
		A int
		B int `inject:"by=A"`
	}
	err := inject.AddResolver(func(state *TagFnState, a int) (int, error) {
		if state.CtxData == nil {
			return 0, errors.New("ctx data is nil")
		}
		base, ok := state.CtxData["base"].(int)
		if !ok {
			return 0, errors.New("base is not provided")
		}
		return base + a, nil
	})
	if !assert.NoError(t, err) {
		return
	}
	inject.CacheForStruct(&input{})
	param := &input{
		A: 1,
	}
	err = inject.StructWithCtxData(param, map[string]interface{}{"base": 10})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, 11, param.B) {
		return
	}
}

func TestInjectByNestedField(t *testing.T) {
	inject := New(&Config{
		TagName:      "inject",
		FieldNameTag: "",
	})
	type input struct {
		Username string
	}
	type input2 struct {
		C    input
		User *user `inject:"by=C.Username"`
	}
	err := inject.AddResolver(userByUsername)
	if err != nil {
		panic(err)
	}
	inject.CacheForStruct(&input2{})
	param := &input2{
		C: input{
			Username: "peter",
		},
	}
	err = inject.Struct(param)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, param.User) {
		return
	}
	if !assert.Equal(t, "peter", param.User.Username) {
		return
	}
}
