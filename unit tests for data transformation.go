package transformation

import (
	"encoding/json"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestSourceFieldTransformationDetail_GetFieldTransformationDetail(t *testing.T) {
	tests := []struct {
		name           string
		sf             SourceFieldTransformationDetail
		fieldName      string
		expectedError  bool
		expectedField  string
	}{
		{
			name: "Valid field name",
			sf: SourceFieldTransformationDetail{
				FieldTransformationDetails: map[string]FieldTransformationDetail{
					"test_key": {
						FieldName: "name",
					},
				},
			},
			fieldName:     "name",
			expectedError: false,
		},
		{
			name: "Invalid field name",
			sf: SourceFieldTransformationDetail{
				FieldTransformationDetails: map[string]FieldTransformationDetail{
					"test_key": {
						FieldName: "name",
					},
				},
			},
			fieldName:     "nonexistent",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail, err := tt.sf.GetFieldTransformationDetail(tt.fieldName)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, detail)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, detail)
				assert.Equal(t, tt.fieldName, detail.FieldName)
			}
		})
	}
}

func TestFieldTransformationDetail_InitializeTransformFunctions(t *testing.T) {
	tests := []struct {
		name          string
		detail        FieldTransformationDetail
		expectedError bool
	}{
		{
			name: "Valid transformation functions",
			detail: FieldTransformationDetail{
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name: "Concat",
						Type: "Concat",
					},
					{
						Name: "JMESPath",
						Type: "JMESPath",
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Invalid transformation function",
			detail: FieldTransformationDetail{
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name: "NonexistentFunction",
						Type: "NonexistentFunction",
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.detail.InitializeTransformFunctions()
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for _, detail := range tt.detail.TransformFunctionDetails {
					assert.NotNil(t, detail.TransformationFunction)
				}
			}
		})
	}
}

func TestSourceFieldTransformationDetail_TransformRawText(t *testing.T) {
	tests := []struct {
		name           string
		sf             SourceFieldTransformationDetail
		variables      map[string]string
		expected       string
		expectedError  bool
	}{
		{
			name: "Simple variable substitution",
			sf: SourceFieldTransformationDetail{
				RawText: "Hello, ${name}!",
			},
			variables: map[string]string{
				"name": "John",
			},
			expected:      "Hello, John!",
			expectedError: false,
		},
		{
			name: "Missing variable",
			sf: SourceFieldTransformationDetail{
				RawText: "Hello, ${name}!",
			},
			variables:     map[string]string{},
			expected:      "",
			expectedError: true,
		},
		{
			name: "Complex transformation",
			sf: SourceFieldTransformationDetail{
				RawText: "Hello, ${name}! Transformed: f{name}",
				FieldTransformationDetails: map[string]FieldTransformationDetail{
					"test_key": {
						FieldName: "name",
						TransformFunctionDetails: []TransformationFunctionDetail{
							{
								Name: "Concat",
								Type: "Concat",
								Content: map[string]interface{}{
									"prefix": "prefix_",
									"suffix": "_suffix",
								},
							},
						},
					},
				},
			},
			variables: map[string]string{
				"name": "John",
			},
			expected:      "Hello, John! Transformed: prefix_John_suffix",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.sf.TransformRawText(tt.variables)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDataTransformationDetail_GetTransformedVariables(t *testing.T) {
	// Create test data
	testData := DataTransformationDetail{
		"job1": map[string]interface{}{
			"input_transformation": map[string]interface{}{
				"source1": map[string]interface{}{
					"raw_text":     "Hello, ${name}!",
					"target_field": "greeting",
					"fields": map[string]interface{}{
						"test_key": map[string]interface{}{
							"field_name": "name",
							"functions": []interface{}{
								map[string]interface{}{
									"name":    "Concat",
									"type":    "Concat",
									"content": map[string]interface{}{
										"prefix": "prefix_",
										"suffix": "_suffix",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		dt             DataTransformationDetail
		jobID          string
		variables      map[string]string
		expected       map[string]string
		expectedError  bool
	}{
		{
			name: "Valid transformation",
			dt:   testData,
			jobID: "job1",
			variables: map[string]string{
				"name": "John",
			},
			expected: map[string]string{
				"greeting": "Hello, John!",
			},
			expectedError: false,
		},
		{
			name:   "Invalid job ID",
			dt:     testData,
			jobID:  "nonexistent",
			variables: map[string]string{
				"name": "John",
			},
			expected:      nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.dt.GetTransformedVariables(tt.jobID, tt.variables)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestFieldTransformationDetail_ApplyTransformFunctions tests the application of transformation functions
func TestFieldTransformationDetail_ApplyTransformFunctions(t *testing.T) {
	tests := []struct {
		name           string
		detail         FieldTransformationDetail
		input          string
		expected       string
		expectedError  bool
	}{
		{
			name: "Single Concat transformation",
			detail: FieldTransformationDetail{
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 0,
						Content: map[string]interface{}{
							"prefix": "prefix_",
							"suffix": "_suffix",
						},
					},
				},
			},
			input:         "test",
			expected:      "prefix_test_suffix",
			expectedError: false,
		},
		{
			name: "Multiple ordered transformations",
			detail: FieldTransformationDetail{
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 1,
						Content: map[string]interface{}{
							"prefix": "prefix_",
							"suffix": "_suffix",
						},
					},
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 0,
						Content: map[string]interface{}{
							"prefix": "first_",
							"suffix": "_last",
						},
					},
				},
			},
			input:         "test",
			expected:      "prefix_first_test_last_suffix",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.detail.InitializeTransformFunctions()
			assert.NoError(t, err)

			result, err := tt.detail.ApplyTransformFunctions(tt.input)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}