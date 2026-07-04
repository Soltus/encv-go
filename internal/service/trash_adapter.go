package service

import (
	"github.com/Soltus/encv-go/internal/tools"
	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// trashMoverAdapter 将 *TrashManagerImpl 适配为 tools.TrashMover 接口。
//
// 原因：tools 包定义了自己的 TrashMover interface 和 TrashItem struct
//（避免循环依赖，tools 不 import service），而 TrashManagerImpl.MoveToTrash
// 返回 tasksystem.TrashItem。本 adapter 做类型转换。
type trashMoverAdapter struct {
	tm *TrashManagerImpl
}

// NewTrashMoverAdapter 创建 tools.TrashMover 适配器。
func NewTrashMoverAdapter(tm *TrashManagerImpl) tools.TrashMover {
	return &trashMoverAdapter{tm: tm}
}

func (a *trashMoverAdapter) MoveToTrash(originalPath string, taskID string) (tools.TrashItem, error) {
	item, err := a.tm.MoveToTrash(originalPath, taskID)
	if err != nil {
		return tools.TrashItem{}, err
	}
	return tools.TrashItem{
		ID:           item.ID,
		OriginalPath: item.OriginalPath,
		TrashPath:    item.TrashPath,
		IsDirectory:  item.IsDirectory,
		Size:         item.Size,
	}, nil
}

// 确保 *trashMoverAdapter 实现 tools.TrashMover 接口。
var _ tools.TrashMover = (*trashMoverAdapter)(nil)

// 确保 *TrashManagerImpl 实现 tasksystem.TrashManager 接口。
var _ tasksystem.TrashManager = (*TrashManagerImpl)(nil)
