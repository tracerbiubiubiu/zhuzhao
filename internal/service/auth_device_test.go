package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// D2-22 守护：device_id 字符白名单 [a-zA-Z0-9_-]{1,64}
func TestValidDeviceID(t *testing.T) {
	assert.True(t, validDeviceID("default"))
	assert.True(t, validDeviceID("550e8400-e29b-41d4-a716-446655440000"), "UUID 标准形态")
	assert.True(t, validDeviceID("dev_1"))
	assert.True(t, validDeviceID("A1"))

	assert.False(t, validDeviceID(""), "空串（空值由 normalizeDeviceID 兜底 default）")
	assert.False(t, validDeviceID("dev 1"), "空格")
	assert.False(t, validDeviceID("dev:1"), "冒号（Redis 键分隔符语义字符）")
	assert.False(t, validDeviceID("dev*"), "通配符")
	assert.False(t, validDeviceID("设备"), "非 ASCII")
	assert.False(t, validDeviceID("a*b*c*d*e*f*g*h*i*j*k*l*m*n*o*p*q*r*s*t*u*v*w*x*y*z*1*2"), "长度 >64")
}
