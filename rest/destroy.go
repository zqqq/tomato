package rest

import "github.com/lfq7413/tomato/orm"

// Destroy ...
type Destroy struct {
	auth         *Auth
	className    string
	query        map[string]interface{}
	originalData map[string]interface{}
}

// NewDestroy ...
func NewDestroy(
	auth *Auth,
	className string,
	query map[string]interface{},
	originalData map[string]interface{},
) *Destroy {
	destroy := &Destroy{
		auth:         auth,
		className:    className,
		query:        query,
		originalData: originalData,
	}
	return destroy
}

// Execute ...
func (d *Destroy) Execute() map[string]interface{} {
	d.handleSession()
	d.runBeforeTrigger()
	d.handleUserRoles()
	d.runDestroy()
	d.runAfterTrigger()
	return nil
}

func (d *Destroy) handleSession() error {
	if d.className != "_Session" {
		return nil
	}
	sessionToken := d.originalData["sessionToken"]
	if sessionToken != nil {
		// TODO 从缓存删除对应的 user
	}

	return nil
}

func (d *Destroy) runBeforeTrigger() error {
	RunTrigger(TypeBeforeDelete, d.className, d.auth, nil, d.originalData)

	return nil
}

func (d *Destroy) handleUserRoles() error {
	if d.auth.IsMaster == false {
		d.auth.GetUserRoles()
	}

	return nil
}

func (d *Destroy) runDestroy() error {
	options := map[string]interface{}{}
	if d.auth.IsMaster == false {
		acl := []string{"*"}
		if d.auth.User != nil {
			acl = append(acl, d.auth.User["objectId"].(string))
			acl = append(acl, d.auth.UserRoles...)
		}
		options["acl"] = acl
	}
	orm.Destroy(d.className, d.query, options)

	return nil
}

func (d *Destroy) runAfterTrigger() error {
	RunTrigger(TypeAfterDelete, d.className, d.auth, nil, d.originalData)

	return nil
}
