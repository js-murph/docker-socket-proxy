package management

// Response represents the standard API response structure
type Response[T any] struct {
	Status   string `json:"status"`
	Response T      `json:"response"`
}

// CreateResponse represents the response from socket creation
type CreateResponse struct {
	Socket string `json:"socket"`
}

// DeleteResponse represents the response from socket deletion
type DeleteResponse struct {
	Message string `json:"message"`
}

// ListResponse represents the response from listing sockets
type ListResponse struct {
	Sockets []string `json:"sockets"`
}

// DescribeResponse represents the response from describing a socket
type DescribeResponse struct {
	Config any `json:"config"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
