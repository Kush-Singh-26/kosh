package utils

var TestingMode = false

func SetTestingMode(val bool) {
	TestingMode = val
}

func IsTestingMode() bool {
	return TestingMode
}
