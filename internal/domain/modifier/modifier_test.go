package modifier

import (
	"testing"
)

func TestUnit_UpsertModifier_GivenValidUpdates_WhenModifying_ThenUpdatesBody(t *testing.T) {
	tests := []struct {
		name     string
		updates  map[string]interface{}
		body     map[string]interface{}
		expected bool
	}{
		{
			"add new field",
			map[string]interface{}{"newField": "newValue"},
			map[string]interface{}{"existingField": "existingValue"},
			true,
		},
		{
			"update existing field",
			map[string]interface{}{"existingField": "updatedValue"},
			map[string]interface{}{"existingField": "originalValue"},
			true,
		},
		{
			"no changes needed",
			map[string]interface{}{"existingField": "originalValue"},
			map[string]interface{}{"existingField": "originalValue"},
			false,
		},
		{
			"empty updates",
			map[string]interface{}{},
			map[string]interface{}{"existingField": "existingValue"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifier := NewUpsertModifier(tt.updates)
			_, result := modifier.Modify(tt.body)
			if result != tt.expected {
				t.Errorf("UpsertModifier.Modify() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_ReplaceModifier_GivenValidUpdates_WhenModifying_ThenReplacesBody(t *testing.T) {
	tests := []struct {
		name     string
		updates  map[string]interface{}
		body     map[string]interface{}
		expected bool
	}{
		{
			"replace existing field",
			map[string]interface{}{"existingField": "newValue"},
			map[string]interface{}{"existingField": "oldValue"},
			true,
		},
		{
			"add new field",
			map[string]interface{}{"newField": "newValue"},
			map[string]interface{}{"existingField": "existingValue"},
			true,
		},
		{
			"no changes needed",
			map[string]interface{}{"existingField": "originalValue"},
			map[string]interface{}{"existingField": "originalValue"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifier := NewReplaceModifier(tt.updates)
			_, result := modifier.Modify(tt.body)
			if result != tt.expected {
				t.Errorf("ReplaceModifier.Modify() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_DeleteModifier_GivenValidFields_WhenModifying_ThenDeletesFields(t *testing.T) {
	tests := []struct {
		name     string
		fields   map[string]interface{}
		body     map[string]interface{}
		expected bool
	}{
		{
			"delete existing field",
			map[string]interface{}{"fieldToDelete": "anyValue"},
			map[string]interface{}{"fieldToDelete": "value", "keepField": "value"},
			true,
		},
		{
			"delete non-existent field",
			map[string]interface{}{"nonExistentField": "anyValue"},
			map[string]interface{}{"existingField": "value"},
			false,
		},
		{
			"delete with matching value",
			map[string]interface{}{"fieldToDelete": "specificValue"},
			map[string]interface{}{"fieldToDelete": "specificValue", "keepField": "value"},
			true,
		},
		{
			"delete with non-matching value",
			map[string]interface{}{"fieldToDelete": "differentValue"},
			map[string]interface{}{"fieldToDelete": "originalValue", "keepField": "value"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifier := NewDeleteModifier(tt.fields)
			_, result := modifier.Modify(tt.body)
			if result != tt.expected {
				t.Errorf("DeleteModifier.Modify() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_CompositeModifier_GivenMultipleModifiers_WhenModifying_ThenAppliesAll(t *testing.T) {
	tests := []struct {
		name      string
		modifiers []Modifier
		body      map[string]interface{}
		expected  bool
	}{
		{
			"multiple modifiers with changes",
			[]Modifier{
				NewUpsertModifier(map[string]interface{}{"field1": "value1"}),
				NewUpsertModifier(map[string]interface{}{"field2": "value2"}),
			},
			map[string]interface{}{"existingField": "existingValue"},
			true,
		},
		{
			"multiple modifiers without changes",
			[]Modifier{
				NewUpsertModifier(map[string]interface{}{"existingField": "existingValue"}),
				NewUpsertModifier(map[string]interface{}{}),
			},
			map[string]interface{}{"existingField": "existingValue"},
			false,
		},
		{
			"empty modifiers",
			[]Modifier{},
			map[string]interface{}{"existingField": "existingValue"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifier := NewCompositeModifier(tt.modifiers...)
			_, result := modifier.Modify(tt.body)
			if result != tt.expected {
				t.Errorf("CompositeModifier.Modify() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_Modifier_GivenNestedObjects_WhenModifying_ThenHandlesCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		updates  map[string]interface{}
		body     map[string]interface{}
		expected bool
	}{
		{
			"upsert nested object",
			map[string]interface{}{
				"nested": map[string]interface{}{
					"field": "value",
				},
			},
			map[string]interface{}{
				"nested": map[string]interface{}{
					"existingField": "existingValue",
				},
			},
			true,
		},
		{
			"replace nested object",
			map[string]interface{}{
				"nested": map[string]interface{}{
					"field": "value",
				},
			},
			map[string]interface{}{
				"nested": map[string]interface{}{
					"existingField": "existingValue",
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upsertModifier := NewUpsertModifier(tt.updates)
			_, upsertResult := upsertModifier.Modify(tt.body)

			// Reset body for replace test
			body := map[string]interface{}{
				"nested": map[string]interface{}{
					"existingField": "existingValue",
				},
			}
			replaceModifier := NewReplaceModifier(tt.updates)
			_, replaceResult := replaceModifier.Modify(body)

			if upsertResult != tt.expected {
				t.Errorf("UpsertModifier.Modify() = %v, want %v", upsertResult, tt.expected)
			}
			if replaceResult != tt.expected {
				t.Errorf("ReplaceModifier.Modify() = %v, want %v", replaceResult, tt.expected)
			}
		})
	}
}

func TestUnit_Modifier_GivenArrays_WhenModifying_ThenHandlesCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		updates  map[string]interface{}
		body     map[string]interface{}
		expected bool
	}{
		{
			"upsert array",
			map[string]interface{}{
				"array": []interface{}{"item1", "item2"},
			},
			map[string]interface{}{
				"array": []interface{}{"existingItem"},
			},
			true,
		},
		{
			"replace array",
			map[string]interface{}{
				"array": []interface{}{"item1", "item2"},
			},
			map[string]interface{}{
				"array": []interface{}{"existingItem"},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upsertModifier := NewUpsertModifier(tt.updates)
			_, upsertResult := upsertModifier.Modify(tt.body)

			// Reset body for replace test
			body := map[string]interface{}{
				"array": []interface{}{"existingItem"},
			}
			replaceModifier := NewReplaceModifier(tt.updates)
			_, replaceResult := replaceModifier.Modify(body)

			if upsertResult != tt.expected {
				t.Errorf("UpsertModifier.Modify() = %v, want %v", upsertResult, tt.expected)
			}
			if replaceResult != tt.expected {
				t.Errorf("ReplaceModifier.Modify() = %v, want %v", replaceResult, tt.expected)
			}
		})
	}
}
