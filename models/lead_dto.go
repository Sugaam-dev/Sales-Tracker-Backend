package models

type CreateLeadRequest struct {
	Company            string `json:"company" binding:"required"`
	ProjectName        string `json:"projectName"`
	Contact            string `json:"contact" binding:"required"`
	Email              string `json:"email" binding:"required,email"`
	Phone              string `json:"phone" binding:"required,len=10,numeric"`
	OfficePhone        string `json:"officePhone" binding:"required,len=10,numeric"`
	OfficePhoneCountry string `json:"officePhoneCountry"`
	Owner              string `json:"owner" binding:"required"`
	Industry           string `json:"industry"`
	Size               string `json:"size"`
	Region             string `json:"region"`
	Source             string `json:"source"`
	Stage              string `json:"stage" binding:"required"`
	Status             string `json:"status" binding:"required"`
	Sentiment          string `json:"sentiment" binding:"required"`
	Priority           string `json:"priority" binding:"required,oneof=Low Normal High Urgent"`
}

type UpdateLeadRequest struct {
	Owner    *string `json:"owner,omitempty"`
	Stage    *string `json:"stage,omitempty"`
	Status   *string `json:"status,omitempty"`
	Priority *string `json:"priority,omitempty" binding:"omitempty,oneof=Low Normal High Urgent"`
	Contact  *string `json:"contact,omitempty"`
	Email    *string `json:"email,omitempty" binding:"omitempty,email"`
	Phone    *string `json:"phone,omitempty" binding:"omitempty,len=10,numeric"`
	Value    *string `json:"value,omitempty"`
}

type LeadResponse struct {
	LeadID      string `json:"id"`
	Company     string `json:"company"`
	ProjectName string `json:"projectName"`
	Contact     string `json:"contact"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	OfficePhone string `json:"officePhone"`
	Owner       string `json:"owner"`
	Stage       string `json:"stage"`
	Status      string `json:"status"`
	Sentiment   string `json:"sentiment"`
	Priority    string `json:"priority"`
	Value       string `json:"value"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func ToLeadResponse(l Lead) LeadResponse {
	valueStr := ""
	if l.Value != nil {
		valueStr = *l.Value
	}

	return LeadResponse{
		LeadID:      l.LeadID,
		Company:     l.Company,
		ProjectName: l.ProjectName,
		Contact:     l.Contact,
		Email:       l.Email,
		Phone:       l.Phone,
		OfficePhone: l.OfficePhone,
		Owner:       l.Owner,
		Stage:       l.Stage,
		Status:      l.Status,
		Sentiment:   l.Sentiment,
		Priority:    l.Priority,
		Value:       valueStr,
		CreatedAt:   l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   l.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type ActivityResponse struct {
	ID        uint   `json:"id"`
	Type      string `json:"type"`
	Desc      string `json:"desc"`
	Outcome   string `json:"outcome"`
	DueDate   string `json:"dueDate"`
	Completed bool   `json:"completed"`
}

func ToActivityResponse(a Activity) ActivityResponse {
	dueDate := ""
	if a.DueDate != nil {
		dueDate = a.DueDate.Format("2006-01-02")
	}
	return ActivityResponse{
		ID:        a.ID,
		Type:      a.Type,
		Desc:      a.Desc,
		Outcome:   a.Outcome,
		DueDate:   dueDate,
		Completed: a.Completed,
	}
}
