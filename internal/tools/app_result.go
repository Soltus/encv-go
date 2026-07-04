// Package tools 提供 AppResult<T> 借鉴自 nuclear-boy AppResult.kt (sealed class)。
//
// Go 习惯用 (value, error) tuple，但核 boy 的 AppResult 提供了 3 个独特的优势：
//   1. 显式 IsSuccess/IsFailure 判别（比 err != nil 更易读）
//   2. Map / OnSuccess / OnFailure 链式组合（避免嵌套 if-err）
//   3. 携带结构化 AppError（不只是字符串）
//
// 此文件提供 struct-based AppResult 作为可选 helper，调用方仍可继续用 (T, error) 风格。
package tools

// AppResult 借鉴自 nuclear-boy AppResult.kt L7-54。
//
// 判别模式：
//   - Success（T != 零值）→ IsSuccess=true
//   - Success 但 T 是零值 → 需配合 Error==nil 判定（典型 case：bool false）
//   - Failure → IsSuccess=false，Error 必有值
type AppResult[T any] struct {
	// Data 成功时的值；失败时为 T 类型的零值。
	Data T
	// Error 失败时的 AppError；成功时为 nil。
	Error *AppError
}

// NewAppResultSuccess 构造成功结果。
func NewAppResultSuccess[T any](data T) AppResult[T] {
	return AppResult[T]{Data: data}
}

// NewAppResultFailure 构造失败结果。
func NewAppResultFailure[T any](ae *AppError) AppResult[T] {
	var zero T
	return AppResult[T]{Data: zero, Error: ae}
}

// IsSuccess 是否成功。
func (r AppResult[T]) IsSuccess() bool { return r.Error == nil }

// IsFailure 是否失败。
func (r AppResult[T]) IsFailure() bool { return r.Error != nil }

// GetOrNull 取值或 nil。
func (r AppResult[T]) GetOrNull() T {
	if r.IsFailure() {
		var zero T
		return zero
	}
	return r.Data
}

// GetOrElse 取值或 fallback。
func (r AppResult[T]) GetOrElse(fallback T) T {
	if r.IsFailure() {
		return fallback
	}
	return r.Data
}

// Map 链式转换（Success 走 transform，Failure 直接透传）。
// 借鉴 nuclear-boy AppResult.kt L24-27。
func Map[T, U any](r AppResult[T], transform func(T) U) AppResult[U] {
	if r.IsFailure() {
		return NewAppResultFailure[U](r.Error)
	}
	return NewAppResultSuccess(transform(r.Data))
}

// OnSuccess 副作用钩子（不影响结果，借鉴 nuclear-boy L29-32）。
// 用于打日志/上报指标，不改 Error 状态。
func (r AppResult[T]) OnSuccess(action func(T)) AppResult[T] {
	if r.IsSuccess() {
		action(r.Data)
	}
	return r
}

// OnFailure 副作用钩子（不影响结果，借鉴 nuclear-boy L34-37）。
func (r AppResult[T]) OnFailure(action func(*AppError)) AppResult[T] {
	if r.IsFailure() {
		action(r.Error)
	}
	return r
}

// RunCatching 把可能 panic 的闭包包装为 AppResult。
// 借鉴 nuclear-boy AppResult.kt L44-53 runCatching。
//
// 与 Go 原生 (val, err) 的区别：
//   - 把抛出的 err 升级为 *AppError
//   - *AppError 直接复用；*ToolError 走 FromToolError 升级
//   - 其他 err 包装为 AppErrorUnknown，message = err.Error()
//
// 用法：
//
//	result := tools.RunCatching(func() (string, error) {
//	    return doSomething()
//	})
//	if result.IsFailure() { ... }
//
// 需要更精细的分类（network/timeout/401/5xx 等），调用方在 fn 内部预先调用
// agent.ClassifyException 把 err 转 AppError 再返回。
func RunCatching[T any](fn func() (T, error)) AppResult[T] {
	val, err := fn()
	if err == nil {
		return NewAppResultSuccess(val)
	}
	// *AppError 直接复用
	if ae, ok := err.(*AppError); ok {
		return NewAppResultFailure[T](ae)
	}
	// *ToolError 升级为 AppError
	if te, ok := err.(*ToolError); ok {
		return NewAppResultFailure[T](FromToolError(te))
	}
	// 其他 err 包装为 Unknown
	return NewAppResultFailure[T](NewAppError(AppErrUnknown, err.Error()))
}
