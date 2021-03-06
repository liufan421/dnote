/* Copyright (C) 2019, 2020 Monomax Software Pty Ltd
 *
 * This file is part of Dnote.
 *
 * Dnote is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Dnote is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Dnote.  If not, see <https://www.gnu.org/licenses/>.
 */

package repetition

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/dnote/dnote/pkg/assert"
	"github.com/dnote/dnote/pkg/clock"
	"github.com/dnote/dnote/pkg/server/database"
	"github.com/dnote/dnote/pkg/server/mailer"
	"github.com/dnote/dnote/pkg/server/testutils"
	"github.com/pkg/errors"
)

func assertLastActive(t *testing.T, ruleUUID string, lastActive int64) {
	var rule database.RepetitionRule
	testutils.MustExec(t, testutils.DB.Where("uuid = ?", ruleUUID).First(&rule), "finding rule1")

	assert.Equal(t, rule.LastActive, lastActive, "LastActive mismatch")
}

func assertDigestCount(t *testing.T, rule database.RepetitionRule, expected int) {
	var digestCount int
	testutils.MustExec(t, testutils.DB.Model(&database.Digest{}).Where("rule_id = ? AND user_id = ?", rule.ID, rule.UserID).Count(&digestCount), "counting digest")
	assert.Equal(t, digestCount, expected, "digest count mismatch")
}

func getTestContext(c clock.Clock, be *testutils.MockEmailbackendImplementation) Context {
	emailTmplDir := os.Getenv("DNOTE_TEST_EMAIL_TEMPLATE_DIR")

	return Context{
		DB:           testutils.DB,
		Clock:        c,
		EmailTmpl:    mailer.NewTemplates(&emailTmplDir),
		EmailBackend: be,
	}
}

func mustDo(t *testing.T, c Context) {
	_, err := Do(c)
	if err != nil {
		t.Fatal(errors.Wrap(err, "performing"))
	}
}

func TestDo(t *testing.T) {
	t.Run("processes the rule on time", func(t *testing.T) {
		defer testutils.ClearData()

		// Set up
		user := testutils.SetupUserData()
		a := testutils.SetupAccountData(user, "alice@example.com", "pass1234")
		testutils.MustExec(t, testutils.DB.Model(&a).Update("email_verified", true), "updating email_verified")

		t0 := time.Date(2009, time.November, 1, 0, 0, 0, 0, time.UTC)
		t1 := time.Date(2009, time.November, 4, 12, 2, 0, 0, time.UTC)
		r1 := database.RepetitionRule{
			Title:      "Rule 1",
			Frequency:  (time.Hour * 24 * 3).Milliseconds(), // three days
			Hour:       12,
			Minute:     2,
			Enabled:    true,
			LastActive: 0,
			NextActive: t1.UnixNano() / int64(time.Millisecond),
			UserID:     user.ID,
			BookDomain: database.BookDomainAll,
			Model: database.Model{
				CreatedAt: t0,
				UpdatedAt: t0,
			},
		}

		testutils.MustExec(t, testutils.DB.Save(&r1), "preparing rule1")

		c := clock.NewMock()
		be := testutils.MockEmailbackendImplementation{}
		con := getTestContext(c, &be)

		// Test
		// 1 day later
		c.SetNow(time.Date(2009, time.November, 2, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(0))
		assertDigestCount(t, r1, 0)
		assert.Equalf(t, len(be.Emails), 0, "email queue count mismatch")

		// 2 days later
		c.SetNow(time.Date(2009, time.November, 3, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(0))
		assertDigestCount(t, r1, 0)
		assert.Equal(t, len(be.Emails), 0, "email queue count mismatch")

		// 3 days later - should be processed
		c.SetNow(time.Date(2009, time.November, 4, 12, 1, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(0))
		assertDigestCount(t, r1, 0)
		assert.Equal(t, len(be.Emails), 0, "email queue count mismatch")

		c.SetNow(time.Date(2009, time.November, 4, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257336120000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		c.SetNow(time.Date(2009, time.November, 4, 12, 3, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257336120000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		// 4 day later
		c.SetNow(time.Date(2009, time.November, 5, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257336120000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		// 5 days later
		c.SetNow(time.Date(2009, time.November, 6, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257336120000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		// 6 days later - should be processed
		c.SetNow(time.Date(2009, time.November, 7, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257595320000))
		assertDigestCount(t, r1, 2)
		assert.Equal(t, len(be.Emails), 2, "email queue count mismatch")

		// 7 days later
		c.SetNow(time.Date(2009, time.November, 8, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257595320000))
		assertDigestCount(t, r1, 2)
		assert.Equal(t, len(be.Emails), 2, "email queue count mismatch")

		// 8 days later
		c.SetNow(time.Date(2009, time.November, 9, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257595320000))
		assertDigestCount(t, r1, 2)
		assert.Equal(t, len(be.Emails), 2, "email queue count mismatch")

		// 9 days later - should be processed
		c.SetNow(time.Date(2009, time.November, 10, 12, 2, 1, 0, time.UTC))
		mustDo(t, con)
		assertLastActive(t, r1.UUID, int64(1257854520000))
		assertDigestCount(t, r1, 3)
		assert.Equal(t, len(be.Emails), 3, "email queue count mismatch")
	})

	/*
	* |----|----|----|----|----|----|----|----|----|----|----|----|----|
	* t0             t1        td   t2        tu   t3             t4
	*
	* Suppose a repetition with a frequency of 3 days.
	*
	* t0 - original last_active value (Nov 1, 2009)
	* t1 - original next_active value (Nov 4, 2009)
	* td - server goes down
	* t2 - repetition processing is missed (Nov 7, 2009)
	* tu - server comes up
	* t3 - new last_active value (Nov 10, 2009)
	* t4 - new next_active value (Nov 13, 2009)
	 */
	t.Run("recovers correct next_active value if missed processing in the past", func(t *testing.T) {
		defer testutils.ClearData()

		// Set up
		user := testutils.SetupUserData()
		a := testutils.SetupAccountData(user, "alice@example.com", "pass1234")
		testutils.MustExec(t, testutils.DB.Model(&a).Update("email_verified", true), "updating email_verified")

		t0 := time.Date(2009, time.November, 1, 12, 2, 0, 0, time.UTC)
		t1 := time.Date(2009, time.November, 4, 12, 2, 0, 0, time.UTC)
		r1 := database.RepetitionRule{
			Title:      "Rule 1",
			Frequency:  (time.Hour * 24 * 3).Milliseconds(), // three days
			Hour:       12,
			Minute:     2,
			Enabled:    true,
			LastActive: t0.UnixNano() / int64(time.Millisecond),
			NextActive: t1.UnixNano() / int64(time.Millisecond),
			UserID:     user.ID,
			BookDomain: database.BookDomainAll,
			Model: database.Model{
				CreatedAt: t0,
				UpdatedAt: t0,
			},
		}

		testutils.MustExec(t, testutils.DB.Save(&r1), "preparing rule1")

		c := clock.NewMock()
		c.SetNow(time.Date(2009, time.November, 10, 12, 2, 1, 0, time.UTC))
		be := &testutils.MockEmailbackendImplementation{}

		mustDo(t, getTestContext(c, be))

		var rule database.RepetitionRule
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", r1.UUID).First(&rule), "finding rule1")

		assert.Equal(t, rule.LastActive, time.Date(2009, time.November, 10, 12, 2, 0, 0, time.UTC).UnixNano()/int64(time.Millisecond), "LastActive mismsatch")
		assert.Equal(t, rule.NextActive, time.Date(2009, time.November, 13, 12, 2, 0, 0, time.UTC).UnixNano()/int64(time.Millisecond), "NextActive mismsatch")
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")
	})
}

func TestDo_Disabled(t *testing.T) {
	defer testutils.ClearData()

	// Set up
	user := testutils.SetupUserData()
	a := testutils.SetupAccountData(user, "alice@example.com", "pass1234")
	testutils.MustExec(t, testutils.DB.Model(&a).Update("email_verified", true), "updating email_verified")

	t0 := time.Date(2009, time.November, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2009, time.November, 4, 12, 2, 0, 0, time.UTC)
	r1 := database.RepetitionRule{
		Title:      "Rule 1",
		Frequency:  (time.Hour * 24 * 3).Milliseconds(), // three days
		Hour:       12,
		Minute:     2,
		LastActive: 0,
		NextActive: t1.UnixNano() / int64(time.Millisecond),
		UserID:     user.ID,
		Enabled:    false,
		BookDomain: database.BookDomainAll,
		Model: database.Model{
			CreatedAt: t0,
			UpdatedAt: t0,
		},
	}

	testutils.MustExec(t, testutils.DB.Save(&r1), "preparing rule1")

	// Execute
	c := clock.NewMock()
	c.SetNow(time.Date(2009, time.November, 4, 12, 2, 0, 0, time.UTC))
	be := &testutils.MockEmailbackendImplementation{}

	mustDo(t, getTestContext(c, be))

	// Test
	assertLastActive(t, r1.UUID, int64(0))
	assertDigestCount(t, r1, 0)
	assert.Equal(t, len(be.Emails), 0, "email queue count mismatch")
}

func TestDo_BalancedStrategy(t *testing.T) {
	type testData struct {
		User  database.User
		Book1 database.Book
		Book2 database.Book
		Book3 database.Book
		Note1 database.Note
		Note2 database.Note
		Note3 database.Note
	}

	setup := func() testData {
		user := testutils.SetupUserData()
		a := testutils.SetupAccountData(user, "alice@example.com", "pass1234")
		testutils.MustExec(t, testutils.DB.Model(&a).Update("email_verified", true), "updating email_verified")

		b1 := database.Book{
			UserID: user.ID,
			Label:  "js",
		}
		testutils.MustExec(t, testutils.DB.Save(&b1), "preparing b1")
		b2 := database.Book{
			UserID: user.ID,
			Label:  "css",
		}
		testutils.MustExec(t, testutils.DB.Save(&b2), "preparing b2")
		b3 := database.Book{
			UserID: user.ID,
			Label:  "golang",
		}
		testutils.MustExec(t, testutils.DB.Save(&b3), "preparing b3")

		n1 := database.Note{
			UserID:   user.ID,
			BookUUID: b1.UUID,
		}
		testutils.MustExec(t, testutils.DB.Save(&n1), "preparing n1")
		n2 := database.Note{
			UserID:   user.ID,
			BookUUID: b2.UUID,
		}
		testutils.MustExec(t, testutils.DB.Save(&n2), "preparing n2")
		n3 := database.Note{
			UserID:   user.ID,
			BookUUID: b3.UUID,
		}
		testutils.MustExec(t, testutils.DB.Save(&n3), "preparing n3")

		return testData{
			User:  user,
			Book1: b1,
			Book2: b2,
			Book3: b3,
			Note1: n1,
			Note2: n2,
			Note3: n3,
		}
	}

	t.Run("all books", func(t *testing.T) {
		defer testutils.ClearData()

		// Set up
		dat := setup()

		t0 := time.Date(2009, time.November, 1, 12, 0, 0, 0, time.UTC)
		t1 := time.Date(2009, time.November, 8, 21, 0, 0, 0, time.UTC)
		r1 := database.RepetitionRule{
			Title:      "Rule 1",
			Frequency:  (time.Hour * 24 * 7).Milliseconds(),
			Hour:       21,
			Minute:     0,
			LastActive: 0,
			NextActive: t1.UnixNano() / int64(time.Millisecond),
			Enabled:    true,
			UserID:     dat.User.ID,
			BookDomain: database.BookDomainAll,
			NoteCount:  5,
			Model: database.Model{
				CreatedAt: t0,
				UpdatedAt: t0,
			},
		}
		testutils.MustExec(t, testutils.DB.Save(&r1), "preparing rule1")

		// Execute
		c := clock.NewMock()
		c.SetNow(time.Date(2009, time.November, 8, 21, 0, 0, 0, time.UTC))
		be := &testutils.MockEmailbackendImplementation{}

		mustDo(t, getTestContext(c, be))

		// Test
		assertLastActive(t, r1.UUID, int64(1257714000000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		var repetition database.Digest
		testutils.MustExec(t, testutils.DB.Where("rule_id = ? AND user_id = ?", r1.ID, r1.UserID).Preload("Notes").First(&repetition), "finding repetition")

		sort.SliceStable(repetition.Notes, func(i, j int) bool {
			n1 := repetition.Notes[i]
			n2 := repetition.Notes[j]

			return n1.ID < n2.ID
		})

		var n1Record, n2Record, n3Record database.Note
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note1.UUID).First(&n1Record), "finding n1")
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note2.UUID).First(&n2Record), "finding n2")
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note3.UUID).First(&n3Record), "finding n3")
		expected := []database.Note{n1Record, n2Record, n3Record}
		assert.DeepEqual(t, repetition.Notes, expected, "result mismatch")
	})

	t.Run("excluding books", func(t *testing.T) {
		defer testutils.ClearData()

		// Set up
		dat := setup()

		t0 := time.Date(2009, time.November, 1, 12, 0, 0, 0, time.UTC)
		t1 := time.Date(2009, time.November, 8, 21, 0, 0, 0, time.UTC)
		r1 := database.RepetitionRule{
			Title:      "Rule 1",
			Frequency:  (time.Hour * 24 * 7).Milliseconds(),
			Hour:       21,
			Enabled:    true,
			Minute:     0,
			LastActive: 0,
			NextActive: t1.UnixNano() / int64(time.Millisecond),
			UserID:     dat.User.ID,
			BookDomain: database.BookDomainExluding,
			Books:      []database.Book{dat.Book1},
			NoteCount:  5,
			Model: database.Model{
				CreatedAt: t0,
				UpdatedAt: t0,
			},
		}
		testutils.MustExec(t, testutils.DB.Save(&r1), "preparing rule1")

		// Execute
		c := clock.NewMock()
		c.SetNow(time.Date(2009, time.November, 8, 21, 0, 1, 0, time.UTC))
		be := &testutils.MockEmailbackendImplementation{}

		mustDo(t, getTestContext(c, be))

		// Test
		assertLastActive(t, r1.UUID, int64(1257714000000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		var repetition database.Digest
		testutils.MustExec(t, testutils.DB.Where("rule_id = ? AND user_id = ?", r1.ID, r1.UserID).Preload("Notes").First(&repetition), "finding repetition")

		sort.SliceStable(repetition.Notes, func(i, j int) bool {
			n1 := repetition.Notes[i]
			n2 := repetition.Notes[j]

			return n1.ID < n2.ID
		})

		var n2Record, n3Record database.Note
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note2.UUID).First(&n2Record), "finding n2")
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note3.UUID).First(&n3Record), "finding n3")
		expected := []database.Note{n2Record, n3Record}
		assert.DeepEqual(t, repetition.Notes, expected, "result mismatch")
	})

	t.Run("including books", func(t *testing.T) {
		defer testutils.ClearData()

		// Set up
		dat := setup()

		t0 := time.Date(2009, time.November, 1, 12, 0, 0, 0, time.UTC)
		t1 := time.Date(2009, time.November, 8, 21, 0, 0, 0, time.UTC)
		r1 := database.RepetitionRule{
			Title:      "Rule 1",
			Frequency:  (time.Hour * 24 * 7).Milliseconds(),
			Hour:       21,
			Enabled:    true,
			Minute:     0,
			LastActive: 0,
			NextActive: t1.UnixNano() / int64(time.Millisecond),
			UserID:     dat.User.ID,
			BookDomain: database.BookDomainIncluding,
			Books:      []database.Book{dat.Book1, dat.Book2},
			NoteCount:  5,
			Model: database.Model{
				CreatedAt: t0,
				UpdatedAt: t0,
			},
		}
		testutils.MustExec(t, testutils.DB.Save(&r1), "preparing rule1")

		// Execute
		c := clock.NewMock()
		c.SetNow(time.Date(2009, time.November, 8, 21, 0, 0, 0, time.UTC))
		be := &testutils.MockEmailbackendImplementation{}

		mustDo(t, getTestContext(c, be))

		// Test
		assertLastActive(t, r1.UUID, int64(1257714000000))
		assertDigestCount(t, r1, 1)
		assert.Equal(t, len(be.Emails), 1, "email queue count mismatch")

		var repetition database.Digest
		testutils.MustExec(t, testutils.DB.Where("rule_id = ? AND user_id = ?", r1.ID, r1.UserID).Preload("Notes").First(&repetition), "finding repetition")

		sort.SliceStable(repetition.Notes, func(i, j int) bool {
			n1 := repetition.Notes[i]
			n2 := repetition.Notes[j]

			return n1.ID < n2.ID
		})

		var n1Record, n2Record database.Note
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note1.UUID).First(&n1Record), "finding n1")
		testutils.MustExec(t, testutils.DB.Where("uuid = ?", dat.Note2.UUID).First(&n2Record), "finding n2")
		expected := []database.Note{n1Record, n2Record}
		assert.DeepEqual(t, repetition.Notes, expected, "result mismatch")
	})
}
