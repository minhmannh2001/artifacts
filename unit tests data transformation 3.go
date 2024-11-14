package transformation

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestFieldTransformationDetail_DeepValidation(t *testing.T) {
	tests := []struct {
		name           string
		detail         FieldTransformationDetail
		input          string
		expected       string
		expectedError  bool
		validateFields func(t *testing.T, detail *FieldTransformationDetail)
	}{
		{
			name: "Complex Concat Chain",
			detail: FieldTransformationDetail{
				FieldName: "test_field",
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 0,
						Content: map[string]interface{}{
							"prefix": "prefix_",
							"suffix": "_middle",
						},
					},
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 1,
						Content: map[string]interface{}{
							"prefix": "start_",
							"suffix": "_end",
						},
					},
				},
			},
			input:    "test",
			expected: "start_prefix_test_middle_end",
			validateFields: func(t *testing.T, detail *FieldTransformationDetail) {
				assert.Equal(t, "test_field", detail.FieldName)
				assert.Len(t, detail.TransformFunctionDetails, 2)
				
				// Validate first transformation
				assert.Equal(t, "Concat", detail.TransformFunctionDetails[0].Name)
				assert.Equal(t, 0, detail.TransformFunctionDetails[0].Index)
				content0 := detail.TransformFunctionDetails[0].Content.(map[string]interface{})
				assert.Equal(t, "prefix_", content0["prefix"])
				assert.Equal(t, "_middle", content0["suffix"])
				
				// Validate second transformation
				assert.Equal(t, "Concat", detail.TransformFunctionDetails[1].Name)
				assert.Equal(t, 1, detail.TransformFunctionDetails[1].Index)
				content1 := detail.TransformFunctionDetails[1].Content.(map[string]interface{})
				assert.Equal(t, "start_", content1["prefix"])
				assert.Equal(t, "_end", content1["suffix"])
			},
		},
		{
			name: "Complex JMESPath Transformation",
			detail: FieldTransformationDetail{
				FieldName: "nested_field",
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "JMESPath",
						Type:  "JMESPath",
						Index: 0,
						Content: map[string]interface{}{
							"value": map[string]interface{}{
								"expression_path": "person.details.name",
							},
						},
					},
				},
			},
			input: `{"person":{"details":{"name":"John"}}}`,
			expected: "John",
			validateFields: func(t *testing.T, detail *FieldTransformationDetail) {
				assert.Equal(t, "nested_field", detail.FieldName)
				assert.Len(t, detail.TransformFunctionDetails, 1)
				
				transform := detail.TransformFunctionDetails[0]
				assert.Equal(t, "JMESPath", transform.Name)
				assert.Equal(t, "JMESPath", transform.Type)
				
				content := transform.Content.(map[string]interface{})
				value := content["value"].(map[string]interface{})
				assert.Equal(t, "person.details.name", value["expression_path"])
			},
		},
		{
			name: "Complex RegexExtract Chain",
			detail: FieldTransformationDetail{
				FieldName: "regex_field",
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "RegexExtract",
						Type:  "RegexExtract",
						Index: 0,
						Content: map[string]interface{}{
							"value": map[string]interface{}{
								"pattern": "\\d+",
							},
						},
					},
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 1,
						Content: map[string]interface{}{
							"prefix": "ID:",
							"suffix": "",
						},
					},
				},
			},
			input: "User123 logged in",
			expected: "ID:123",
			validateFields: func(t *testing.T, detail *FieldTransformationDetail) {
				assert.Equal(t, "regex_field", detail.FieldName)
				assert.Len(t, detail.TransformFunctionDetails, 2)
				
				// Validate regex transformation
				regex := detail.TransformFunctionDetails[0]
				assert.Equal(t, "RegexExtract", regex.Name)
				regexContent := regex.Content.(map[string]interface{})
				regexValue := regexContent["value"].(map[string]interface{})
				assert.Equal(t, "\\d+", regexValue["pattern"])
				
				// Validate concat transformation
				concat := detail.TransformFunctionDetails[1]
				assert.Equal(t, "Concat", concat.Name)
				concatContent := concat.Content.(map[string]interface{})
				assert.Equal(t, "ID:", concatContent["prefix"])
			},
		},
		{
			name: "Complex Value Transformation Chain",
			detail: FieldTransformationDetail{
				FieldName: "value_transform_field",
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "ValueTransformation",
						Type:  "ValueTransformation",
						Index: 0,
						Content: map[string]interface{}{
							"rules": []map[string]interface{}{
								{
									"type": "VALUE_TO_VALUE",
									"value": map[string]interface{}{
										"input":  []string{"apple", "banana"},
										"mapped": "fruit",
									},
								},
								{
									"type": "RANGE_TO_VALUE",
									"value": map[string]interface{}{
										"from":   "10",
										"to":     "20",
										"mapped": "teen",
									},
								},
							},
						},
					},
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 1,
						Content: map[string]interface{}{
							"prefix": "Category: ",
							"suffix": "!",
						},
					},
				},
			},
			input: "apple",
			expected: "Category: fruit!",
			validateFields: func(t *testing.T, detail *FieldTransformationDetail) {
				assert.Equal(t, "value_transform_field", detail.FieldName)
				assert.Len(t, detail.TransformFunctionDetails, 2)
				
				// Validate value transformation
				valueTransform := detail.TransformFunctionDetails[0]
				assert.Equal(t, "ValueTransformation", valueTransform.Name)
				content := valueTransform.Content.(map[string]interface{})
				rules := content["rules"].([]map[string]interface{})
				
				// Validate VALUE_TO_VALUE rule
				rule1 := rules[0]
				assert.Equal(t, "VALUE_TO_VALUE", rule1["type"])
				value1 := rule1["value"].(map[string]interface{})
				assert.Equal(t, []string{"apple", "banana"}, value1["input"])
				assert.Equal(t, "fruit", value1["mapped"])
				
				// Validate RANGE_TO_VALUE rule
				rule2 := rules[1]
				assert.Equal(t, "RANGE_TO_VALUE", rule2["type"])
				value2 := rule2["value"].(map[string]interface{})
				assert.Equal(t, "10", value2["from"])
				assert.Equal(t, "20", value2["to"])
				assert.Equal(t, "teen", value2["mapped"])
			},
		},
		{
			name: "All Transformations Combined",
			detail: FieldTransformationDetail{
				FieldName: "combined_field",
				TransformFunctionDetails: []TransformationFunctionDetail{
					{
						Name:  "JMESPath",
						Type:  "JMESPath",
						Index: 0,
						Content: map[string]interface{}{
							"value": map[string]interface{}{
								"expression_path": "user.id",
							},
						},
					},
					{
						Name:  "RegexExtract",
						Type:  "RegexExtract",
						Index: 1,
						Content: map[string]interface{}{
							"value": map[string]interface{}{
								"pattern": "\\d+",
							},
						},
					},
					{
						Name:  "ValueTransformation",
						Type:  "ValueTransformation",
						Index: 2,
						Content: map[string]interface{}{
							"rules": []map[string]interface{}{
								{
									"type": "RANGE_TO_VALUE",
									"value": map[string]interface{}{
										"from":   "1",
										"to":     "100",
										"mapped": "valid",
									},
								},
							},
						},
					},
					{
						Name:  "Concat",
						Type:  "Concat",
						Index: 3,
						Content: map[string]interface{}{
							"prefix": "Status: ",
							"suffix": "!",
						},
					},
				},
			},
			input: `{"user":{"id":"USER42"}}`,
			expected: "Status: valid!",
			validateFields: func(t *testing.T, detail *FieldTransformationDetail) {
				assert.Equal(t, "combined_field", detail.FieldName)
				assert.Len(t, detail.TransformFunctionDetails, 4)
				
				// Validate correct ordering by index
				transforms := detail.TransformFunctionDetails
				assert.Equal(t, "JMESPath", transforms[0].Name)
				assert.Equal(t, "RegexExtract", transforms[1].Name)
				assert.Equal(t, "ValueTransformation", transforms[2].Name)
				assert.Equal(t, "Concat", transforms[3].Name)
				
				// Validate indices
				for i, transform := range transforms {
					assert.Equal(t, i, transform.Index)
				}
				
				// Validate that each transformation has required content
				for _, transform := range transforms {
					assert.NotNil(t, transform.Content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize transformation functions
			err := tt.detail.InitializeTransformFunctions()
			assert.NoError(t, err)
			
			// Validate fields
			if tt.validateFields != nil {
				tt.validateFields(t, &tt.detail)
			}
			
			// Test transformation
			result, err := tt.detail.ApplyTransformFunctions(tt.input)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
			
			// Validate that transformations maintain their order after execution
			for i, transform := range tt.detail.TransformFunctionDetails {
				assert.Equal(t, i, transform.Index, "Transformation order should be maintained")
			}
		})
	}
}