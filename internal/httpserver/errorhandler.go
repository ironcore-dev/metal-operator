package httpserver

import (
	"context"
	"errors"
	"net/http"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Status struct {
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type StatusRecognizerBuilder struct {
	funcs []func(err error) (status any, code int, ok bool)
}

func NewStatusRecognizerBuilder() *StatusRecognizerBuilder {
	return &StatusRecognizerBuilder{}
}

func (r *StatusRecognizerBuilder) OnFunc(match func(error) (error, bool), f func(error) (status any, code int)) *StatusRecognizerBuilder {
	r.funcs = append(r.funcs, func(err error) (status any, code int, ok bool) {
		err, ok = match(err)
		if !ok {
			return nil, 0, false
		}

		status, code = f(err)
		return status, code, true
	})
	return r
}

func (r *StatusRecognizerBuilder) On(target error, f func(error) (status any, code int)) *StatusRecognizerBuilder {
	return r.OnFunc(func(err error) (error, bool) {
		if errors.Is(err, target) {
			return err, true
		}
		return nil, false
	}, f)
}

func (r *StatusRecognizerBuilder) OnType[E error](f func(E) (status any, code int)) *StatusRecognizerBuilder {
	return r.OnFunc(func(err error) (error, bool) {
		typed, ok := errors.AsType[E](err)
		if !ok {
			return nil, false
		}
		return typed, false
	}, func(err error) (status any, code int) {
		return f(err.(E))
	})
}

func (r *StatusRecognizerBuilder) Build() StatusRecognizerFunc {
	return func(err error) (status any, code int, ok bool) {
		for _, f := range r.funcs {
			status, code, ok = f(err)
			if ok {
				return status, code, true
			}
		}
		return nil, 0, false
	}
}

type StatusRecognizer interface {
	Recognizes(err error) (status any, code int, ok bool)
}

type StatusRecognizerFunc func(err error) (status any, code int, ok bool)

func (f StatusRecognizerFunc) Recognizes(err error) (status any, code int, ok bool) {
	return f(err)
}

type StatusTransformer interface {
	ToStatus(ctx context.Context, err error) (status any, code int)
}

type statusTransformer struct {
	defaultStatus func() any
	defaultCode   int
	recognizer    StatusRecognizer
}

type StatusTransformerOptions struct {
	DefaultStatus func() any
	DefaultCode   int
}

type StatusTransformerOption func(*StatusTransformerOptions)

func NewStatusTransformer(recognizer StatusRecognizer, opts ...StatusTransformerOption) StatusTransformer {
	sOpts := &StatusTransformerOptions{}
	for _, opt := range opts {
		opt(sOpts)
	}

	if sOpts.DefaultCode == 0 {
		sOpts.DefaultCode = http.StatusInternalServerError
	}
	if sOpts.DefaultStatus == nil {
		sOpts.DefaultStatus = func() any {
			return &Status{
				Message: http.StatusText(sOpts.DefaultCode),
			}
		}
	}
	return &statusTransformer{
		defaultStatus: sOpts.DefaultStatus,
		defaultCode:   sOpts.DefaultCode,
		recognizer:    recognizer,
	}
}

func (s *statusTransformer) ToStatus(ctx context.Context, err error) (status any, code int) {
	if status, code, ok := s.recognizer.Recognizes(err); ok {
		return status, code
	}
	log := logf.FromContext(ctx)
	log.Error(err, "Unhandled error")
	return s.defaultStatus(), s.defaultCode
}

type TransformingErrorHandler struct {
	statusTransformer StatusTransformer
}

type ErrorHandler interface {
	Handle(w http.ResponseWriter, req *http.Request, err error)
}

func NewTransformingErrorHandler(statusTransformer StatusTransformer) ErrorHandler {
	return &TransformingErrorHandler{
		statusTransformer: statusTransformer,
	}
}

type RespondWithStatusOption func(error, *Status)

func RespondWithStatus(code int, opts ...RespondWithStatusOption) func(error) (any, int) {
	return func(err error) (any, int) {
		s := &Status{}
		for _, opt := range opts {
			opt(err, s)
		}
		if s.Message == "" {
			s.Message = err.Error()
		}
		return s, code
	}
}

func (h *TransformingErrorHandler) Handle(w http.ResponseWriter, req *http.Request, err error) {
	status, code := h.statusTransformer.ToStatus(req.Context(), err)
	if status != nil {
		WriteJSON(w, status, code)
	} else {
		w.WriteHeader(code)
	}
}

var DefaultErrorHandler = NewTransformingErrorHandler(
	NewStatusTransformer(
		NewStatusRecognizerBuilder().
			On(ErrForbidden, RespondWithStatus(http.StatusForbidden)).
			On(ErrNotFound, RespondWithStatus(http.StatusNotFound)).
			On(ErrBadRequest, RespondWithStatus(http.StatusBadRequest)).
			Build()))
