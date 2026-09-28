package node

import "github.com/scdoproject/scan-api/database"

//NodeDB interface to access node db
type NodeDB interface {
	AddNodeInfo(nodeInfo *database.DBNodeInfo) error
	DeleteNodeInfo(nodeInfo *database.DBNodeInfo) error
	GetNodeInfoByID(id string) (*database.DBNodeInfo, error)
	GetNodeInfos() ([]*database.DBNodeInfo, error)
}
