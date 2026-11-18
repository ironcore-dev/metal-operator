package ptrs

func DerefOr[E any](e *E, def E) E {
	if e != nil {
		return *e
	}
	return def
}

func DerefOrZero[E any](e *E) E {
	if e != nil {
		return *e
	}
	var zero E
	return zero
}

func Equal[E comparable](a, b *E) bool {
	aNil := a == nil
	bNil := b == nil
	if aNil != bNil {
		return false
	}
	if aNil {
		return true
	}
	return *a == *b
}

func EqualFunc[E any](a, b *E, equal func(a, b E) bool) bool {
	aNil := a == nil
	bNil := b == nil
	if aNil != bNil {
		return false
	}
	if aNil {
		return true
	}
	return equal(*a, *b)
}
