package transformation

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestDataTransformationDetail_GetInputTransformationDetail(t *testing.T) {
	tests := []struct {
		name          string
		dt            DataTransformationDetail
		jobID         string
		expectedError string
		shouldBeNil   bool
	}{
		{
			name: "Valid input transformation detail",
			dt: DataTransformationDetail{
				"job1": map[string]interface{}{
					"input_transformation": map[string]interface{}{
						"source1": map[string]interface{}{
							"raw_text": "Hello ${name}",
							"fields": map[string]interface{}{
								"field1": map[string]interface{}{
									"field_name": "name",
								},
							},
						},
					},
				},
			},
			jobID:         "job1",
			expectedError: "",
			shouldBeNil:   false,
		},
		{
			name: "Job ID not found",
			dt: DataTransformationDetail{
				"job1": map[string]interface{}{},
			},
			jobID:         "nonexistent",
			expectedError: "job ID 'nonexistent' not found in data transformation",
			shouldBeNil:   true,
		},
		{
			name: "Invalid job data type",
			dt: DataTransformationDetail{
				"job1": "invalid_type", // Not a map[string]interface{}
			},
			jobID:         "job1",
			expectedError: "job ID 'job1' data is not of type map[string]interface{}, got string",
			shouldBeNil:   true,
		},
		{
			name: "Missing input_transformation field",
			dt: DataTransformationDetail{
				"job1": map[string]interface{}{
					"other_field": "value",
				},
			},
			jobID:         "job1",
			expectedError: "input_transformation field not found for job ID 'job1'",
			shouldBeNil:   true,
		},
		{
			name: "Invalid input_transformation structure",
			dt: DataTransformationDetail{
				"job1": map[string]interface{}{
					"input_transformation": map[string]interface{}{
						"source1": "invalid", // Should be a map[string]interface{}
					},
				},
			},
			jobID:         "job1",
			expectedError: "failed to unmarshal input_transformation for job ID 'job1' into InputTransformationDetail",
			shouldBeNil:   true,
		},
		{
			name: "Complex nested structure",
			dt: DataTransformationDetail{
				"job1": map[string]interface{}{
					"input_transformation": map[string]interface{}{
						"source1": map[string]interface{}{
							"raw_text": "Hello ${name}",
							"fields": map[string]interface{}{
								"field1": map[string]interface{}{
									"field_name": "name",
									"functions": []interface{}{
										map[string]interface{}{
											"name": "Concat",
											"type": "Concat",
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
			},
			jobID:         "job1",
			expectedError: "",
			shouldBeNil:   false,
		},
		{
			name: "Empty input transformation",
			dt: DataTransformationDetail{
				"job1": map[string]interface{}{
					"input_transformation": map[string]interface{}{},
				},
			},
			jobID:         "job1",
			expectedError: "",
			shouldBeNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.dt.getInputTransformationDetail(tt.jobID)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				if tt.shouldBeNil {
					assert.Nil(t, result)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			// Additional validation for successful cases
			if tt.expectedError == "" {
				// Verify the structure is correctly unmarshaled
				if dt, ok := tt.dt[tt.jobID].(map[string]interface{}); ok {
					if inputTransformation, ok := dt["input_transformation"].(map[string]interface{}); ok {
						// Check if the number of sources matches
						assert.Equal(t, len(inputTransformation), len(result))

						// For each source in the original data, verify it exists in the result
						for sourceName := range inputTransformation {
							_, exists := result[sourceName]
							assert.True(t, exists, "Source %s should exist in the result", sourceName)
						}
					}
				}
			}
		})
	}
}

// TestDataTransformationDetail_GetInputTransformationDetail_DeepValidation performs deeper validation
// of the unmarshaled structure
func TestDataTransformationDetail_GetInputTransformationDetail_DeepValidation(t *testing.T) {
	// Test case with a complete transformation structure
	dt := DataTransformationDetail{
		"job1": map[string]interface{}{
			"input_transformation": map[string]interface{}{
				"source1": map[string]interface{}{
					"raw_text":     "Hello ${name}",
					"target_field": "greeting",
					"fields": map[string]interface{}{
						"field1": map[string]interface{}{
							"field_name": "name",
							"functions": []interface{}{
								map[string]interface{}{
									"name":  "Concat",
									"type":  "Concat",
									"index": 0,
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

	result, err := dt.getInputTransformationDetail("job1")
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Validate the structure of the result
	source1, exists := result["source1"]
	assert.True(t, exists)
	assert.Equal(t, "Hello ${name}", source1.RawText)
	assert.Equal(t, "greeting", source1.TargetField)

	// Validate fields
	assert.NotEmpty(t, source1.FieldTransformationDetails)
	for _, fieldDetail := range source1.FieldTransformationDetails {
		assert.Equal(t, "name", fieldDetail.FieldName)
		assert.NotEmpty(t, fieldDetail.TransformFunctionDetails)

		// Validate function details
		for _, funcDetail := range fieldDetail.TransformFunctionDetails {
			assert.Equal(t, "Concat", funcDetail.Name)
			assert.Equal(t, "Concat", funcDetail.Type)
			assert.Equal(t, 0, funcDetail.Index)

			// Validate content
			content, ok := funcDetail.Content.(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, "prefix_", content["prefix"])
			assert.Equal(t, "_suffix", content["suffix"])
		}
	}
}