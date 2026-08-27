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

type CreateActivityRequest struct {
	Type    string `json:"type" binding:"required,oneof=Call Email Demo Other"`
	Desc    string `json:"desc" binding:"required"`
	Outcome string `json:"outcome"`
	DueDate string `json:"dueDate" binding:"omitempty,datetime=2006-01-02"`
}

type CompleteActivityRequest struct {
	Completed bool `json:"completed"`
}

type BulkCreateLeadsRequest struct {
	Leads []CreateLeadRequest `json:"leads" binding:"required,min=1,max=200,dive"`
}

type BulkCreateCreatedResponse struct {
	LeadID  string `json:"leadId"`
	Company string `json:"company"`
}

type BulkCreateFailedResponse struct {
	Index   int               `json:"index"`
	Company string            `json:"company"`
	Errors  map[string]string `json:"errors"`
}

type BulkCreateSummary struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Failed  int `json:"failed"`
}

type BulkCreateResponse struct {
	Success bool                        `json:"success"`
	Summary BulkCreateSummary           `json:"summary"`
	Created []BulkCreateCreatedResponse `json:"created"`
	Failed  []BulkCreateFailedResponse  `json:"failed"`
}
