package cron

import (
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAndSetLastShowsUpdateTime(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	// Initial check: no update recorded
	ts, err := GetLastShowsUpdateTime(db)
	require.NoError(t, err)
	assert.True(t, ts.IsZero(), "expected zero time when no update has been recorded")

	// Set timestamp
	now := time.Now().Truncate(time.Second)
	err = SetLastShowsUpdateTime(db, now)
	require.NoError(t, err)

	// Read back
	retrieved, err := GetLastShowsUpdateTime(db)
	require.NoError(t, err)
	assert.Equal(t, now.Unix(), retrieved.Unix())

	// Update timestamp again (overwrite)
	later := now.Add(2 * time.Hour)
	err = SetLastShowsUpdateTime(db, later)
	require.NoError(t, err)

	retrievedLater, err := GetLastShowsUpdateTime(db)
	require.NoError(t, err)
	assert.Equal(t, later.Unix(), retrievedLater.Unix())
}

func TestGetAndSetLastShowsUpdateTime_NilDB(t *testing.T) {
	ts, err := GetLastShowsUpdateTime(nil)
	assert.NoError(t, err)
	assert.True(t, ts.IsZero())

	err = SetLastShowsUpdateTime(nil, time.Now())
	assert.NoError(t, err)
}
