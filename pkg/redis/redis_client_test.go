package redis_test

import (
	"errors"
	"github.com/alicebob/miniredis"
	"github.com/applike/gosoline/pkg/mon/mocks"
	"github.com/applike/gosoline/pkg/redis"
	"github.com/elliotchance/redismock"
	baseRedis "github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRedisBLPop(t *testing.T) {
	r, c := buildClient()

	if _, err := r.Lpush("list", "value"); err != nil {
		panic(err)
	}

	res, err := c.BLPop(1*time.Second, "list")

	assert.Nil(t, err, "there should be no error on blpop")
	assert.Equal(t, "value", res[1])
}

func TestRedisDel(t *testing.T) {
	_, c := buildClient()

	count, err := c.Del("test")
	assert.Nil(t, err, "there should be no error on Del")
	assert.Equal(t, 0, int(count))

	var ttl time.Duration
	err = c.Set("key", "value", ttl)
	assert.Nil(t, err, "there should be no error on Del")

	count, err = c.Del("key")
	assert.Nil(t, err, "there should be no error on Del")
	assert.Equal(t, 1, int(count))
}

func TestRedisLLen(t *testing.T) {
	s, c := buildClient()

	for i := 0; i < 3; i++ {
		if _, err := s.Lpush("list", "value"); err != nil {
			panic(err)
		}
	}

	res, err := c.LLen("list")

	assert.Nil(t, err, "there should be no error on LLen")
	assert.Equal(t, int64(3), res)
}

func TestRedisRPush(t *testing.T) {
	_, c := buildClient()

	count, err := c.RPush("list", "v1", "v2", "v3")
	assert.Nil(t, err, "there should be no error on RPush")
	assert.Equal(t, int64(3), count)
}

func TestRedisSet(t *testing.T) {
	_, c := buildClient()

	var ttl time.Duration
	err := c.Set("key", "value", ttl)
	assert.Nil(t, err, "there should be no error on Set")

	ttl, _ = time.ParseDuration("1m")
	err = c.Set("key", "value", ttl)
	assert.Nil(t, err, "there should be no error on Set with expiration date")
}

func TestRedisSetWithOOM(t *testing.T) {
	var ttl time.Duration

	settings := redis.Settings{}
	writer := mocks.NewMetricWriterMockedAll()
	redisMock := redismock.NewMock()

	redisMock.On("Set").Return(baseRedis.NewStatusResult("", errors.New("OOM command not allowed when used memory > 'maxmemory'"))).Once()
	redisMock.On("Set").Return(baseRedis.NewStatusResult("", nil)).Once()

	logger := mocks.NewLoggerMockedAll()
	c := redis.NewRedisClientWithInterfaces(redisMock, logger, writer, &settings)

	err := c.Set("key", "value", ttl)

	assert.Nil(t, err, "there should be no error on Set with backoff")
	redisMock.AssertExpectations(t)
}

func TestRedisSetWithError(t *testing.T) {
	var ttl time.Duration

	settings := redis.Settings{}
	writer := mocks.NewMetricWriterMockedAll()
	redisMock := redismock.NewMock()

	redisMock.On("Set").Return(baseRedis.NewStatusResult("", errors.New("random redis error"))).Once()
	redisMock.On("Set").Return(baseRedis.NewStatusResult("", nil)).Times(0)

	logger := mocks.NewLoggerMockedAll()
	c := redis.NewRedisClientWithInterfaces(redisMock, logger, writer, &settings)

	err := c.Set("key", "value", ttl)

	assert.NotNil(t, err, "there should be an error on Set")
	redisMock.AssertExpectations(t)
}

func buildClient() (*miniredis.Miniredis, redis.Client) {
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}

	settings := redis.Settings{}
	settings.Address = s.Addr()
	settings.Mode = redis.RedisModeLocal
	logger := mocks.NewLoggerMockedAll()
	c := redis.GetClientFromSettings(logger, &settings)

	return s, c
}
