// Package errors 定义后台统一使用的业务错误类型。
package errors

// BusinessError 表示可以安全返回给前端的业务错误。
//
// 参数说明：
// - CodeValue：稳定错误码，供前端或日志检索使用。
// - StatusValue：HTTP 状态码。
// - MessageValue：可展示给用户的中文错误信息。
type BusinessError struct {
	CodeValue    string
	StatusValue  int
	MessageValue string
}

// Error 返回错误文本。
func (e BusinessError) Error() string {
	return e.MessageValue
}

// Code 返回稳定错误码。
func (e BusinessError) Code() string {
	return e.CodeValue
}

// StatusCode 返回 HTTP 状态码。
func (e BusinessError) StatusCode() int {
	return e.StatusValue
}

// PublicMessage 返回可展示给用户的错误信息。
func (e BusinessError) PublicMessage() string {
	return e.MessageValue
}

// New 创建业务错误。
//
// 参数说明：
// - code：稳定错误码。
// - status：HTTP 状态码。
// - message：前端可展示的中文提示。
func New(code string, status int, message string) BusinessError {
	return BusinessError{CodeValue: code, StatusValue: status, MessageValue: message}
}
