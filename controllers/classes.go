package controllers

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/astaxie/beego"
	"github.com/lfq7413/tomato/client"
	"github.com/lfq7413/tomato/config"
	"github.com/lfq7413/tomato/errs"
	"github.com/lfq7413/tomato/rest"
	"github.com/lfq7413/tomato/types"
	"github.com/lfq7413/tomato/utils"
)

// ClassesController 对象操作 API 的基础结构
// 处理 /classes 接口的所有请求，处理内部类的部分请求
// Info 当前请求的权限信息
// Auth 当前请求的用户权限
// JSONBody 由 JSON 格式转换来的请求数据
// RawBody 原始请求数据
// ClassName 要操作的类名
// ObjectID 要操作的对象 id
type ClassesController struct {
	beego.Controller
	Info      *RequestInfo
	Auth      *rest.Auth
	JSONBody  types.M
	RawBody   []byte
	ClassName string
	ObjectID  string
}

// RequestInfo http 请求的权限信息
type RequestInfo struct {
	AppID          string
	MasterKey      string
	ClientKey      string
	SessionToken   string
	InstallationID string
	ClientVersion  string
	ClientSDK      map[string]string
}

// Prepare 对请求权限进行处理
// 1. 从请求头中获取各种 key
// 2. 尝试按 json 格式转换 body
// 3. 尝试从 body 中获取各种 key
// 4. 校验请求权限
// 5. 生成用户信息
func (o *ClassesController) Prepare() {
	info := &RequestInfo{}
	info.AppID = o.Ctx.Input.Header("X-Parse-Application-Id")
	info.MasterKey = o.Ctx.Input.Header("X-Parse-Master-Key")
	info.ClientKey = o.Ctx.Input.Header("X-Parse-Client-Key")
	info.SessionToken = o.Ctx.Input.Header("X-Parse-Session-Token")
	info.InstallationID = o.Ctx.Input.Header("X-Parse-Installation-Id")
	info.ClientVersion = o.Ctx.Input.Header("X-Parse-Client-Version")

	basicAuth := httpAuth(o.Ctx.Input.Header("authorization"))
	if basicAuth != nil {
		info.AppID = basicAuth["appId"]
		if basicAuth["masterKey"] != "" {
			info.MasterKey = basicAuth["masterKey"]
		}
		if basicAuth["javascriptKey"] != "" {
			info.ClientKey = basicAuth["javascriptKey"]
		}
	}

	if o.Ctx.Input.RequestBody != nil {
		contentType := o.Ctx.Input.Header("Content-type")
		if strings.HasPrefix(contentType, "application/json") {
			// 请求数据为 json 格式，进行转换，转换出错则返回错误
			var object types.M
			err := json.Unmarshal(o.Ctx.Input.RequestBody, &object)
			if err != nil {
				o.Data["json"] = errs.ErrorMessageToMap(errs.InvalidJSON, "invalid JSON")
				o.ServeJSON()
				return
			}
			o.JSONBody = object
		} else {
			// TODO 转换 json 之前，可能需要判断一下数据大小，以确保不会去转换超大数据
			// 其他格式的请求数据，仅尝试转换，转换失败不返回错误
			var object types.M
			err := json.Unmarshal(o.Ctx.Input.RequestBody, &object)
			if err != nil {
				o.RawBody = o.Ctx.Input.RequestBody
			} else {
				o.JSONBody = object
			}
		}
	}

	if o.JSONBody != nil {
		// Unity SDK sends a _noBody key which needs to be removed.
		// Unclear at this point if action needs to be taken.
		delete(o.JSONBody, "_noBody")

		delete(o.JSONBody, "_RevocableSession")
	}

	if info.AppID == "" {
		if o.JSONBody != nil {
			delete(o.JSONBody, "_RevocableSession")
		}
		// 从请求数据中获取各种 key
		if o.JSONBody != nil && o.JSONBody["_ApplicationId"] != nil {
			info.AppID = o.JSONBody["_ApplicationId"].(string)
			delete(o.JSONBody, "_ApplicationId")
			if o.JSONBody["_ClientKey"] != nil {
				info.ClientKey = o.JSONBody["_ClientKey"].(string)
				delete(o.JSONBody, "_ClientKey")
			}
			if o.JSONBody["_InstallationId"] != nil {
				info.InstallationID = o.JSONBody["_InstallationId"].(string)
				delete(o.JSONBody, "_InstallationId")
			}
			if o.JSONBody["_SessionToken"] != nil {
				info.SessionToken = o.JSONBody["_SessionToken"].(string)
				delete(o.JSONBody, "_SessionToken")
			}
			if o.JSONBody["_MasterKey"] != nil {
				info.MasterKey = o.JSONBody["_MasterKey"].(string)
				delete(o.JSONBody, "_MasterKey")
			}
			if o.JSONBody["_ContentType"] != nil {
				o.Ctx.Input.Context.Request.Header.Set("Content-type", o.JSONBody["_ContentType"].(string))
				delete(o.JSONBody, "_ContentType")
			}
		} else {
			// 请求数据中也不存在 APPID 时，返回错误
			o.Data["json"] = errs.ErrorMessageToMap(403, "unauthorized")
			o.Ctx.Output.SetStatus(403)
			o.ServeJSON()
			return
		}
	}

	if info.ClientVersion != "" {
		info.ClientSDK = client.FromString(info.ClientVersion)
	}

	if o.JSONBody != nil && o.JSONBody["base64"] != nil {
		// 请求数据中存在 base64 字段，表明为文件上传，解码并设置到 RawBody 上
		data, err := base64.StdEncoding.DecodeString(o.JSONBody["base64"].(string))
		if err == nil {
			o.RawBody = data
		}
	}

	o.Info = info

	// 校验请求权限
	if info.AppID != config.TConfig.AppID {
		o.Data["json"] = errs.ErrorMessageToMap(403, "unauthorized")
		o.Ctx.Output.SetStatus(403)
		o.ServeJSON()
		return
	}
	if info.MasterKey == config.TConfig.MasterKey {
		o.Auth = &rest.Auth{InstallationID: info.InstallationID, IsMaster: true}
		return
	}
	if info.ClientKey != config.TConfig.ClientKey {
		o.Data["json"] = errs.ErrorMessageToMap(403, "unauthorized")
		o.Ctx.Output.SetStatus(403)
		o.ServeJSON()
		return
	}
	// 登录时删除 Token
	url := o.Ctx.Input.URL()
	if strings.HasSuffix(url, "/login/") {
		info.SessionToken = ""
	}
	// 生成当前会话用户权限信息
	if info.SessionToken == "" {
		o.Auth = &rest.Auth{InstallationID: info.InstallationID, IsMaster: false}
	} else {
		var err error
		o.Auth, err = rest.GetAuthForSessionToken(info.SessionToken, info.InstallationID)
		if err != nil {
			o.Data["json"] = errs.ErrorToMap(err)
			o.ServeJSON()
			return
		}
	}
}

func httpAuth(authorization string) map[string]string {
	if authorization == "" {
		return nil
	}

	header := authorization
	var appID, masterKey, javascriptKey string
	authPrefix := "basic "

	match := strings.HasPrefix(strings.ToLower(header), authPrefix)
	if match {
		encodedAuth := header[len(authPrefix):len(header)]
		credentials := strings.Split(decodeBase64(encodedAuth), ":")

		if len(credentials) == 2 {
			appID = credentials[0]
			key := credentials[1]
			jsKeyPrefix := "javascript-key="

			matchKey := strings.HasPrefix(key, jsKeyPrefix)
			if matchKey {
				javascriptKey = key[len(jsKeyPrefix):len(key)]
			} else {
				masterKey = key
			}
			return map[string]string{
				"appId":         appID,
				"masterKey":     masterKey,
				"javascriptKey": javascriptKey,
			}
		}
		return nil
	}

	return nil
}

func decodeBase64(str string) string {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return ""
	}
	return string(data)
}

// HandleCreate 处理对象创建请求，返回对象 id 与对象位置
// @router /:className [post]
func (o *ClassesController) HandleCreate() {

	if o.ClassName == "" {
		o.ClassName = o.Ctx.Input.Param(":className")
	}

	if o.JSONBody == nil {
		o.Data["json"] = errs.ErrorMessageToMap(errs.InvalidJSON, "request body is empty")
		o.ServeJSON()
		return
	}

	result, err := rest.Create(o.Auth, o.ClassName, o.JSONBody, o.Info.ClientSDK)
	if err != nil {
		o.Data["json"] = errs.ErrorToMap(err)
		o.ServeJSON()
		return
	}

	o.Data["json"] = result["response"]
	o.Ctx.Output.SetStatus(201)
	o.Ctx.Output.Header("location", result["location"].(string))
	o.ServeJSON()

}

// HandleGet 处理查询指定对象请求，返回查询到的对象
// @router /:className/:objectId [get]
func (o *ClassesController) HandleGet() {
	if o.ClassName == "" {
		o.ClassName = o.Ctx.Input.Param(":className")
	}
	if o.ObjectID == "" {
		o.ObjectID = o.Ctx.Input.Param(":objectId")
	}
	options := types.M{}
	if o.GetString("keys") != "" {
		options["keys"] = o.GetString("keys")
	}
	if o.GetString("include") != "" {
		options["include"] = o.GetString("include")
	}
	response, err := rest.Get(o.Auth, o.ClassName, o.ObjectID, options, o.Info.ClientSDK)

	if err != nil {
		o.Data["json"] = errs.ErrorToMap(err)
		o.ServeJSON()
		return
	}

	results := utils.A(response["results"])
	if results == nil && len(results) == 0 {
		o.Data["json"] = errs.ErrorMessageToMap(errs.ObjectNotFound, "Object not found.")
		o.ServeJSON()
		return
	}

	result := utils.M(results[0])

	if o.ClassName == "_User" {
		delete(result, "sessionToken")
		if o.Auth.User != nil && result["objectId"].(string) == o.Auth.User["objectId"].(string) {
			// 重新设置 session token
			result["sessionToken"] = o.Info.SessionToken
		}
	}

	o.Data["json"] = result
	o.ServeJSON()

}

// HandleUpdate 处理更新指定对象请求
// @router /:className/:objectId [put]
func (o *ClassesController) HandleUpdate() {

	if o.ClassName == "" {
		o.ClassName = o.Ctx.Input.Param(":className")
	}
	if o.ObjectID == "" {
		o.ObjectID = o.Ctx.Input.Param(":objectId")
	}

	if o.JSONBody == nil {
		o.Data["json"] = errs.ErrorMessageToMap(errs.InvalidJSON, "request body is empty")
		o.ServeJSON()
		return
	}

	result, err := rest.Update(o.Auth, o.ClassName, o.ObjectID, o.JSONBody, o.Info.ClientSDK)
	if err != nil {
		o.Data["json"] = errs.ErrorToMap(err)
		o.ServeJSON()
		return
	}

	o.Data["json"] = result["response"]
	o.ServeJSON()

}

// HandleFind 处理查找对象请求
// @router /:className [get]
func (o *ClassesController) HandleFind() {
	if o.ClassName == "" {
		o.ClassName = o.Ctx.Input.Param(":className")
	}

	// 获取查询参数，并组装
	options := types.M{}
	if o.GetString("skip") != "" {
		if i, err := strconv.Atoi(o.GetString("skip")); err == nil {
			options["skip"] = i
		} else {
			o.Data["json"] = errs.ErrorMessageToMap(errs.InvalidQuery, "skip should be int")
			o.ServeJSON()
			return
		}
	}
	if o.GetString("limit") != "" {
		if i, err := strconv.Atoi(o.GetString("limit")); err == nil {
			options["limit"] = i
		} else {
			o.Data["json"] = errs.ErrorMessageToMap(errs.InvalidQuery, "limit should be int")
			o.ServeJSON()
			return
		}
	} else {
		options["limit"] = 100
	}
	if o.GetString("order") != "" {
		options["order"] = o.GetString("order")
	}
	if o.GetString("count") != "" {
		options["count"] = true
	}
	if o.GetString("keys") != "" {
		options["keys"] = o.GetString("keys")
	}
	if o.GetString("include") != "" {
		options["include"] = o.GetString("include")
	}
	if o.GetString("redirectClassNameForKey") != "" {
		options["redirectClassNameForKey"] = o.GetString("redirectClassNameForKey")
	}

	where := types.M{}
	if o.GetString("where") != "" {
		err := json.Unmarshal([]byte(o.GetString("where")), &where)
		if err != nil {
			o.Data["json"] = errs.ErrorMessageToMap(errs.InvalidJSON, "where should be valid json")
			o.ServeJSON()
			return
		}
	}

	response, err := rest.Find(o.Auth, o.ClassName, where, options, o.Info.ClientSDK)
	if err != nil {
		o.Data["json"] = errs.ErrorToMap(err)
		o.ServeJSON()
		return
	}
	if utils.HasResults(response) {
		results := utils.A(response["results"])
		for _, v := range results {
			result := utils.M(v)
			if result["sessionToken"] != nil && o.Info.SessionToken != "" {
				result["sessionToken"] = o.Info.SessionToken
			}
		}
	}

	o.Data["json"] = response
	o.ServeJSON()
}

// HandleDelete 处理删除指定对象请求
// @router /:className/:objectId [delete]
func (o *ClassesController) HandleDelete() {

	if o.ClassName == "" {
		o.ClassName = o.Ctx.Input.Param(":className")
	}
	if o.ObjectID == "" {
		o.ObjectID = o.Ctx.Input.Param(":objectId")
	}

	err := rest.Delete(o.Auth, o.ClassName, o.ObjectID, o.Info.ClientSDK)
	if err != nil {
		o.Data["json"] = errs.ErrorToMap(err)
		o.ServeJSON()
		return
	}

	o.Data["json"] = types.M{}
	o.ServeJSON()
}

// Get ...
// @router / [get]
func (o *ClassesController) Get() {
	o.Ctx.Output.SetStatus(405)
	o.Data["json"] = errs.ErrorMessageToMap(405, "Method Not Allowed")
	o.ServeJSON()
}

// Post ...
// @router / [post]
func (o *ClassesController) Post() {
	o.Ctx.Output.SetStatus(405)
	o.Data["json"] = errs.ErrorMessageToMap(405, "Method Not Allowed")
	o.ServeJSON()
}

// Delete ...
// @router / [delete]
func (o *ClassesController) Delete() {
	o.Ctx.Output.SetStatus(405)
	o.Data["json"] = errs.ErrorMessageToMap(405, "Method Not Allowed")
	o.ServeJSON()
}

// Put ...
// @router / [put]
func (o *ClassesController) Put() {
	o.Ctx.Output.SetStatus(405)
	o.Data["json"] = errs.ErrorMessageToMap(405, "Method Not Allowed")
	o.ServeJSON()
}