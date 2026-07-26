package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// checkBatchAccess enforces that a mentor/employee caller only reads or
// mutates content scoped to a batch they actually manage (batch_manager_id or
// additional_manager_id) — the same scoping already applied to list endpoints
// via FindAllForMentor, now extended to detail/update/delete/grade handlers
// that were previously gated by role alone. college_admin/college_staff are
// scoped the same way but by college ownership instead of manager identity —
// this is what makes it safe to let them use these same content-authoring
// routes without leaking or letting them touch another college's data.
// team_lead/super_admin and any content with no batch (global, batchShortID
// == "") pass through unchanged.
//
// Writes the error response itself on failure; the caller should return
// immediately when this returns false.
func checkBatchAccess(c *gin.Context, batchRepo *repository.BatchRepository, batchShortID string) bool {
	role := c.GetString("role")
	scoped := role == string(models.RoleMentor) || role == string(models.RoleEmployee) ||
		role == string(models.RoleCollegeAdmin) || role == string(models.RoleCollegeStaff)
	if !scoped {
		return true
	}
	if batchShortID == "" {
		return true
	}

	batch, err := batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify batch access"})
		return false
	}

	if role == string(models.RoleCollegeAdmin) || role == string(models.RoleCollegeStaff) {
		collegeID := c.GetString("college_id")
		owns := batch != nil && collegeID != "" && batch.CollegeID == collegeID
		if !owns {
			c.JSON(http.StatusForbidden, gin.H{"error": "this batch does not belong to your college"})
			return false
		}
		return true
	}

	userID := c.GetString("user_id")
	owns := batch != nil && (batch.BatchManagerID == userID || (batch.AdditionalManagerID != "" && batch.AdditionalManagerID == userID))
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "you don't manage this batch"})
		return false
	}
	return true
}

// checkCourseAccess enforces that a college_admin/college_staff caller can
// only update or delete a course created for their own college — courses
// have no batch to route through checkBatchAccess, and unlike Update/Delete
// on other resources may be shared across colleges (course_scope, Stage 5),
// so a course with no owning college (CollegeID == "") is treated as global/
// shared and stays off-limits to a college-scoped caller. super_admin/
// team_lead/mentor bypass entirely — Create already resolves the college via
// resolveTargetCollege, so this only needs to cover Update/Delete.
func checkCourseAccess(c *gin.Context, courseRepo *repository.CourseRepository, shortID string) bool {
	role := c.GetString("role")
	if role != string(models.RoleCollegeAdmin) && role != string(models.RoleCollegeStaff) {
		return true
	}

	course, err := courseRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify course access"})
		return false
	}

	collegeID := c.GetString("college_id")
	owns := course != nil && collegeID != "" && course.CollegeID == collegeID
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "this course does not belong to your college"})
		return false
	}
	return true
}

// checkLeadAccess enforces that an employee caller only reads or mutates
// leads assigned to them — super_admin/team_lead bypass (team_lead also
// carries a personal lead quota, but is never restricted from any lead).
// Mirrors checkBatchAccess. Writes the error response itself on failure; the
// caller should return immediately when this returns false.
func checkLeadAccess(c *gin.Context, leadRepo *repository.LeadRepository, leadShortID string) bool {
	role := c.GetString("role")
	if role != string(models.RoleEmployee) {
		return true
	}

	lead, err := leadRepo.FindByShortID(c.Request.Context(), leadShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify lead access"})
		return false
	}

	userID := c.GetString("user_id")
	if lead == nil || lead.AssignedTo != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "this lead isn't assigned to you"})
		return false
	}
	return true
}
