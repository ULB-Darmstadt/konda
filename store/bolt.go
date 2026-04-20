package store

import (
	"encoding/json"
	"time"

	"go.etcd.io/bbolt"
)

type BoltStore struct {
	db     *bbolt.DB
	bucket []byte
}

func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bbolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	bucket := []byte("AppState")
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	})

	return &BoltStore{db: db, bucket: bucket}, err
}

func (s *BoltStore) Get(sessionID string, field FieldKey, value any) error {
	err := s.db.View(func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket)
		sess := root.Bucket([]byte(sessionID))
		if sess == nil {
			return ErrNotFound
		}

		data := sess.Get([]byte(field))
		if data == nil {
			return ErrNotFound
		}

		return json.Unmarshal(data, value)
	})

	// Background update to last accessed
	if err == nil && field != LastAccessedField {
		go func(session string) {
			_ = s.Set(session, LastAccessedField, time.Now())
		}(sessionID)
	}

	return err
}

func (s *BoltStore) Set(sessionID string, field FieldKey, value any) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket)
		sess, err := root.CreateBucketIfNotExists([]byte(sessionID))
		if err != nil {
			return err
		}

		data, err := json.Marshal(value)
		if err != nil {
			return err
		}

		if err := sess.Put([]byte(field), data); err != nil {
			return err
		}

		if field != LastAccessedField {
			ts, _ := json.Marshal(time.Now())
			_ = sess.Put([]byte(LastAccessedField), ts)
		}

		return nil
	})
}

func (s *BoltStore) ModifyField(sessionID string, field FieldKey, modifyFn func(current []byte) ([]byte, error)) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket)
		sess := root.Bucket([]byte(sessionID))
		if sess == nil {
			return ErrNotFound
		}

		old := sess.Get([]byte(field))
		newVal, err := modifyFn(old)
		if err != nil {
			return err
		}

		return sess.Put([]byte(field), newVal)
	})
}

func (s *BoltStore) ForEachField(field FieldKey, fn func(sessionID string, data []byte) error) error {
	return s.db.View(func(tx *bbolt.Tx) error {
		root := tx.Bucket(s.bucket)
		if root == nil {
			return nil
		}

		return root.ForEach(func(k, v []byte) error {
			// We only care about sub-buckets (sessions)
			if v != nil {
				return nil
			}

			sess := root.Bucket(k)
			if sess == nil {
				return nil
			}

			data := sess.Get([]byte(field))
			if data == nil {
				return nil
			}

			return fn(string(k), data)
		})
	})
}

func (s *BoltStore) Delete(sessionID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(s.bucket).DeleteBucket([]byte(sessionID))
	})
}
func (s *BoltStore) Close() error {
	return s.db.Close()
}
