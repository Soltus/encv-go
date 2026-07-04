package com.encvgo.combolite.model

sealed class OperationResult<out T> {
    data class Success<T>(val data: T) : OperationResult<T>()
    data class Failure(val reason: String, val exception: Throwable? = null) : OperationResult<Nothing>()

    inline fun <R> map(transform: (T) -> R): OperationResult<R> = when (this) {
        is Success -> Success(transform(data))
        is Failure -> this
    }

    inline fun onSuccess(action: (T) -> Unit): OperationResult<T> {
        if (this is Success) action(data)
        return this
    }

    inline fun onFailure(action: (String, Throwable?) -> Unit): OperationResult<T> {
        if (this is Failure) action(reason, exception)
        return this
    }
}
