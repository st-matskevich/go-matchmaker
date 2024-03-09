package data

import "github.com/st-matskevich/go-matchmaker/common"

type DataProvider interface {
	Set(req common.RequestBody) (*common.RequestBody, error)
	ListPush(ID string) error
	ListPop() (string, error)
}
