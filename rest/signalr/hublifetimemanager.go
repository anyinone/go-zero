package signalr

import (
	"sync"

	"github.com/go-kit/log"
)

// HubLifetimeManager is a lifetime manager abstraction for hub instances
// OnConnected() is called when a connection is started
// OnDisconnected() is called when a connection is finished
// InvokeAll() sends an invocation message to all hub connections
// InvokeClient() sends an invocation message to a specified hub connection
// InvokeGroup() sends an invocation message to a specified group of hub connections
// AddToGroup() adds a connection to the specified group
// RemoveFromGroup() removes a connection from the specified group
type HubLifetimeManager interface {
	OnConnected(conn hubConnection)
	OnDisconnected(conn hubConnection)
	InvokeAll(target string, args []interface{})
	UserIds() []uint64
	Connections() []ConnectionInfo
	InvokeUser(userId uint64, target string, args []interface{})
	InvokeClient(connectionID string, target string, args []interface{})
	InvokeGroup(groupName string, target string, args []interface{})
	AddToGroup(groupName, connectionID string)
	RemoveFromGroup(groupName, connectionID string)
}

func newLifeTimeManager(info StructuredLogger) defaultHubLifetimeManager {
	return defaultHubLifetimeManager{
		info: log.WithPrefix(info, "ts", log.DefaultTimestampUTC,
			"class", "lifeTimeManager"),
	}
}

type defaultHubLifetimeManager struct {
	clients sync.Map
	groups  sync.Map
	info    StructuredLogger
}

func (d *defaultHubLifetimeManager) OnConnected(conn hubConnection) {
	d.clients.Store(conn.ConnectionID(), conn)
}

func (d *defaultHubLifetimeManager) OnDisconnected(conn hubConnection) {
	d.clients.Delete(conn.ConnectionID())
	d.groups.Range(func(key, value interface{}) bool {
		delete(value.(map[string]hubConnection), conn.ConnectionID())
		return true
	})
}

func (d *defaultHubLifetimeManager) UserIds() []uint64 {
	ids := make([]uint64, 0)
	d.clients.Range(func(key, value interface{}) bool {
		if hub, ok := value.(hubConnection); ok && hub.UserId() > 0 {
			for _, id := range ids {
				if id == hub.UserId() {
					return true
				}
			}
			ids = append(ids, hub.UserId())
		}
		return true
	})
	return ids
}

func (d *defaultHubLifetimeManager) Connections() []ConnectionInfo {
	infos := make([]ConnectionInfo, 0)
	d.clients.Range(func(key, value interface{}) bool {
		if hub, ok := value.(hubConnection); ok {
			infos = append(infos, hub.Information())
		}
		return true
	})
	return infos
}

func (d *defaultHubLifetimeManager) InvokeAll(target string, args []interface{}) {
	d.clients.Range(func(key, value interface{}) bool {
		_ = value.(hubConnection).SendInvocation("", target, args)
		return true
	})
}

func (d *defaultHubLifetimeManager) InvokeUser(userId uint64, target string, args []interface{}) {
	d.clients.Range(func(key, value interface{}) bool {
		if hub, ok := value.(hubConnection); ok && hub.UserId() == userId {
			hub.SendInvocation("", target, args)
		}
		return true
	})
}

func (d *defaultHubLifetimeManager) InvokeClient(connectionID string, target string, args []interface{}) {
	if client, ok := d.clients.Load(connectionID); ok {
		_ = client.(hubConnection).SendInvocation("", target, args)
	}
}

func (d *defaultHubLifetimeManager) InvokeGroup(groupName string, target string, args []interface{}) {
	if groups, ok := d.groups.Load(groupName); ok {
		for _, v := range groups.(map[string]hubConnection) {
			_ = v.SendInvocation("", target, args)
		}
	}
}

func (d *defaultHubLifetimeManager) AddToGroup(groupName string, connectionID string) {
	if client, ok := d.clients.Load(connectionID); ok {
		groups, _ := d.groups.LoadOrStore(groupName, make(map[string]hubConnection))
		groups.(map[string]hubConnection)[connectionID] = client.(hubConnection)
	}
}

func (d *defaultHubLifetimeManager) RemoveFromGroup(groupName string, connectionID string) {
	if groups, ok := d.groups.Load(groupName); ok {
		delete(groups.(map[string]hubConnection), connectionID)
	}
}
