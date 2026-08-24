package file

import "fmt"

const (
	OwnerProduct        = "product"
	OwnerMold           = "mold"
	OwnerWorkOrder      = "workorder"
	OwnerDepartmentTask = "department_task"
)

func validOwnerType(value string) bool {
	switch value {
	case OwnerProduct, OwnerMold, OwnerWorkOrder, OwnerDepartmentTask:
		return true
	}
	return false
}

func ownerError(value string) error { return fmt.Errorf("不支持的 owner_type: %s", value) }
