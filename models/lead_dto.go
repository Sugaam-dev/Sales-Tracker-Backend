package models

type CreateLeadRequest struct {
	Company                  string `json:"company"`
	CompanyName              string `json:"companyName"`
	ProjectName              string `json:"projectName"`
	Contact                  string `json:"contact"`
	LeadName                 string `json:"leadName"`
	Email                    string `json:"email" binding:"required"`
	Phone                    string `json:"phone"`
	ContactNumber            string `json:"contactNumber"`
	CountryCode              string `json:"countryCode"`
	OfficePhone              string `json:"officePhone"`
	OfficePhoneCountry       string `json:"officePhoneCountry"`
	Owner                    string `json:"owner"`
	LeadOwner                string `json:"leadOwner"`
	Industry                 string `json:"industry"`
	Size                     string `json:"size"`
	Region                   string `json:"region"`
	Source                   string `json:"source"`
	Stage                    string `json:"stage"`
	Status                   string `json:"status"`
	LeadStatus               string `json:"leadStatus"`
	Sentiment                string `json:"sentiment"`
	Priority                 string `json:"priority" binding:"required"`
	Designation              string `json:"designation"`
	BestTime                 string `json:"bestTime"`
	Value                    string `json:"value"`
	LostReason               string `json:"lostReason"`
	RequestType              string `json:"requestType" binding:"required"`
	RequestDetails           string `json:"requestDetails" binding:"required"`
	LifecycleTemplate        string `json:"lifecycleTemplate"`
	KamName                  string `json:"kamName"`
	BestTimeToConnect        string `json:"bestTimeToConnect"`
	AlternatePhone           string `json:"alternatePhone"`
	AlternatePhoneCountry    string `json:"alternatePhoneCountry"`
	LinkedinProfileUrl       string `json:"linkedinProfileUrl"`
	LinkedinCompanyPageUrl   string `json:"linkedinCompanyPageUrl"`
	EstimatedRequirementDate string `json:"estimatedRequirementDate"`
	EstimatedReqDate         string `json:"estimatedReqDate"`
	LastContactDate          string `json:"lastContactDate"`
	NextFollowUp             string `json:"nextFollowUp"`
	BasicRequirements        string `json:"basicRequirements"`
	Notes                    string `json:"notes"`
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
