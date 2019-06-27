package metadata

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	contract "github.com/edgexfoundry/go-mod-core-contracts/models"

	"github.com/edgexfoundry/edgex-go/internal/core/metadata/interfaces"
	"github.com/edgexfoundry/edgex-go/internal/core/metadata/interfaces/mocks"
)

// TestURI this is not really used since we are using the HTTP testing framework and not creating routes, but rather
// creating a specific handler which will accept all requests. Therefore, the URI is not important
var TestURI = "/addressable"
var TestAddress = "TestAddress"
var TestPort = 8080

// ErrorPathParam path parameter value which will trigger the 'mux.Vars' function to throw an error due to the '%' not being followed by a valid hexadecimal number.
var ErrorPathParam = "%zz"

// ErrorPortPathParam path parameter used to trigger an error in the `restGetAddressableByPort` function where the port variable is expected to be a number
var ErrorPortPathParam = "abc"

func TestGetAddressablesByAddress(t *testing.T) {
	tests := []struct {
		name           string
		request        *http.Request
		dbMock         interfaces.DBClient
		expectedStatus int
	}{
		{"OK", createRequest(ADDRESS, TestAddress), createMockAddressLoaderForAddressName(1, "GetAddressablesByAddress"), http.StatusOK},
		{"OK(Multiple matches)", createRequest(ADDRESS, TestAddress), createMockAddressLoaderForAddressName(3, "GetAddressablesByAddress"), http.StatusOK},
		{"OK(No matches)", createRequest(ADDRESS, TestAddress), createMockAddressLoaderForAddressName(0, "GetAddressablesByAddress"), http.StatusOK},
		{"Invalid TestAddress path parameter", createRequest(ADDRESS, ErrorPathParam), createMockAddressLoaderForAddressName(1, "GetAddressablesByAddress"), http.StatusBadRequest},
		{"Internal Server Error", createRequest(ADDRESS, TestAddress), createErrorMockAddressLoaderForAddressName("GetAddressablesByAddress"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbClient = tt.dbMock
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(restGetAddressableByAddress)
			handler.ServeHTTP(rr, tt.request)
			response := rr.Result()
			if response.StatusCode != tt.expectedStatus {
				t.Errorf("status code mismatch -- expected %v got %v", tt.expectedStatus, response.StatusCode)
				return
			}
		})
	}
}

func TestGetAddressablesByPublisher(t *testing.T) {
	tests := []struct {
		name           string
		request        *http.Request
		dbMock         interfaces.DBClient
		expectedStatus int
	}{
		{"OK", createRequest(PUBLISHER, TestAddress), createMockAddressLoaderForAddressName(1, "GetAddressablesByPublisher"), http.StatusOK},
		{"OK(Multiple matches)", createRequest(PUBLISHER, TestAddress), createMockAddressLoaderForAddressName(3, "GetAddressablesByPublisher"), http.StatusOK},
		{"OK(No matches)", createRequest(PUBLISHER, TestAddress), createMockAddressLoaderForAddressName(0, "GetAddressablesByPublisher"), http.StatusOK},
		{"Invalid TestAddress path parameter", createRequest(PUBLISHER, ErrorPathParam), createMockAddressLoaderForAddressName(1, "GetAddressablesByPublisher"), http.StatusBadRequest},
		{"Internal Server Error", createRequest(PUBLISHER, TestAddress), createErrorMockAddressLoaderForAddressName("GetAddressablesByPublisher"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbClient = tt.dbMock
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(restGetAddressableByPublisher)
			handler.ServeHTTP(rr, tt.request)
			response := rr.Result()
			if response.StatusCode != tt.expectedStatus {
				t.Errorf("status code mismatch -- expected %v got %v", tt.expectedStatus, response.StatusCode)
				return
			}
		})
	}
}

func TestGetAddressablesByPort(t *testing.T) {
	tests := []struct {
		name           string
		request        *http.Request
		dbMock         interfaces.DBClient
		expectedStatus int
	}{
		{"OK", createRequest(PORT, strconv.Itoa(TestPort)), createMockAddressLoaderForPort(1, "GetAddressablesByPort"), http.StatusOK},
		{"OK(Multiple matches)", createRequest(PORT, strconv.Itoa(TestPort)), createMockAddressLoaderForPort(3, "GetAddressablesByPort"), http.StatusOK},
		{"OK(No matches)", createRequest(PORT, strconv.Itoa(TestPort)), createMockAddressLoaderForPort(0, "GetAddressablesByPort"), http.StatusOK},
		{"Invalid PORT path parameter", createRequest(PORT, ErrorPathParam), createMockAddressLoaderForPort(1, "GetAddressablesByPort"), http.StatusBadRequest},
		{"Non integer PORT path parameter", createRequest(PORT, ErrorPortPathParam), createMockAddressLoaderForPort(1, "GetAddressablesByPort"), http.StatusBadRequest},
		{"Internal Server Error", createRequest(PORT, strconv.Itoa(TestPort)), createErrorMockAddressLoaderPortExecutor("GetAddressablesByPort"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbClient = tt.dbMock
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(restGetAddressableByPort)
			handler.ServeHTTP(rr, tt.request)
			response := rr.Result()
			if response.StatusCode != tt.expectedStatus {
				t.Errorf("status code mismatch -- expected %v got %v", tt.expectedStatus, response.StatusCode)
				return
			}
		})
	}
}

func createRequest(pathParamName string, pathParamValue string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, TestURI, nil)
	return mux.SetURLVars(req, map[string]string{pathParamName: pathParamValue})
}

func createMockAddressLoaderForAddressName(howMany int, methodName string) interfaces.DBClient {
	var addressables []contract.Addressable
	for i := 0; i < howMany; i++ {
		addressables = append(addressables, contract.Addressable{
			User:       "User" + strconv.Itoa(i),
			Protocol:   "http",
			Id:         "address" + strconv.Itoa(i),
			HTTPMethod: "POST",
		})
	}

	myMock := &mocks.DBClient{}
	myMock.On(methodName, TestAddress).Return(addressables, nil)
	return myMock
}

func createErrorMockAddressLoaderForAddressName(methodName string) interfaces.DBClient {
	myMock := &mocks.DBClient{}
	myMock.On(methodName, TestAddress).Return(nil, errors.New("test error"))
	return myMock
}

func createMockAddressLoaderForPort(howMany int, methodName string) interfaces.DBClient {
	var addressables []contract.Addressable
	for i := 0; i < howMany; i++ {
		addressables = append(addressables, contract.Addressable{
			User:       "User" + strconv.Itoa(i),
			Protocol:   "http",
			Id:         "address" + strconv.Itoa(i),
			HTTPMethod: "POST",
		})
	}

	myMock := &mocks.DBClient{}
	myMock.On(methodName, TestPort).Return(addressables, nil)
	return myMock
}

func createErrorMockAddressLoaderPortExecutor(methodName string) interfaces.DBClient {
	myMock := &mocks.DBClient{}
	myMock.On(methodName, TestPort).Return(nil, errors.New("test error"))
	return myMock
}