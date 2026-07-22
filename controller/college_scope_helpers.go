package controller

import (
	"context"
	"errors"

	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// resolveTargetCollege decides which college_id a newly-created batch/
// course/lead/user should belong to, given an optional explicit
// college_short_id from the request body:
//   - super_admin may pick any college via requestedShortID; omitting it
//     defaults to the Internal EdTech Platform.
//   - every other caller is always forced onto their own college_id,
//     regardless of what requestedShortID contains — a College Admin can
//     never provision a record into a different college by passing a
//     different short_id.
func resolveTargetCollege(ctx context.Context, collegeRepo *repository.CollegeRepository, callerRole, callerCollegeID, requestedShortID string) (string, error) {
	if callerRole == string(models.RoleSuperAdmin) {
		if requestedShortID == "" {
			return collegeRepo.DefaultCollegeID(ctx)
		}
		college, err := collegeRepo.FindByShortID(ctx, requestedShortID)
		if err != nil {
			return "", err
		}
		if college == nil {
			return "", errors.New("college not found")
		}
		return college.ID, nil
	}
	if callerCollegeID == "" {
		return "", repository.ErrCollegeScopeRequired
	}
	return callerCollegeID, nil
}
