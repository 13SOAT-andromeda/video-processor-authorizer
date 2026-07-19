package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/13SOAT-andromeda/video-processor-authorizer/pkg/utils"
)

func TestLoggersInitialized(t *testing.T) {
	assert.NotNil(t, utils.InfoLogger)
	assert.NotNil(t, utils.ErrorLogger)
}
