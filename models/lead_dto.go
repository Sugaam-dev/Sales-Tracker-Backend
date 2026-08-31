package models

type CreateLeadRequest struct {
	Company            string `json:"company" binding:"required"`
	ProjectName        string `json:"projectName"`
	Contact            string `json:"contact" binding:"required"`
	Email              string `json:"email"`
	Phone              string `json:"phone" binding:"required"`
	OfficePhone        string `json:"officePhone"`
	OfficePhoneCountry string `json:"officePhoneCountry"`
	Owner              string `json:"owner" binding:"required"`
	Industry           string `json:"industry"`
	Size               string `json:"size"`
	Region             string `json:"region"`
	Source             string `json:"source"`
	Stage              string `json:"stage"`
	Status             string `json:"status" binding:"required"`
	Sentiment          string `json:"sentiment"`
	Priority           string `json:"priority" binding:"required"`
	Designation        string `json:"designation"`
	BestTime           string `json:"bestTime"`
	Value              string `json:"value"`
	LostReason         string `json:"lostReason"`
	ProductService     string `json:"productService" binding:"required"`
	RequestType        string `json:"requestType" binding:"required"`
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
