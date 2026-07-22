package repository

import (
	"errors"

	"github.com/umangagarwal/vedx-backend/models"
)

// ErrCollegeScopeRequired is returned by CollegeFilter when a non-super-admin
// caller has no college_id at all. This should never happen once every
// insert path sets college_id (see CollegeRepository.DefaultCollegeID and
// the college-scoped creation flows) — but if it ever does, the request must
// be rejected rather than silently treated as "see everything," which is
// what today's fail-open RequireFeature bypass does for feature-gating (a
// different, lower-stakes decision than list/detail data segregation).
var ErrCollegeScopeRequired = errors.New("caller has no college scope")

// ErrBatchCollegeMismatch is returned when an operation would place a
// student into a batch belonging to a different college than the student's
// own — e.g. EnrollmentRepository.Transfer. Corresponds to the spec's
// BATCH_COLLEGE_MISMATCH error code.
var ErrBatchCollegeMismatch = errors.New("student and batch belong to different colleges")

// CollegeFilter resolves the college_id a list/search/detail query should be
// scoped to, given the caller's role and their own college_id (as read from
// the JWT-populated Gin context via c.GetString("role")/c.GetString("college_id")).
//
//   - super_admin -> ("", nil): unscoped, see every college's data.
//   - any other role with a blank college_id -> ("", ErrCollegeScopeRequired):
//     reject the request rather than fail open.
//   - everyone else -> (callerCollegeID, nil): scoped to their own college.
func CollegeFilter(callerRole, callerCollegeID string) (string, error) {
	if callerRole == string(models.RoleSuperAdmin) {
		return "", nil
	}
	if callerCollegeID == "" {
		return "", ErrCollegeScopeRequired
	}
	return callerCollegeID, nil
}

