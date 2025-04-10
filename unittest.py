import unittest
from unittest.mock import patch, MagicMock, call
from flask import Flask
from datetime import datetime
import uuid
from helpers.action_logger import Action
from soar_api.rest.alerts import create_alerts_batch

class TestBatchAlertCreation(unittest.TestCase):
    def setUp(self):
        self.app = Flask(__name__)
        self.app.config['TESTING'] = True
        self.app_context = self.app.app_context()
        self.app_context.push()
        
        # Sample test data
        self.valid_alert_data = {
            "severity": "HIGH",
            "message": "Test alert",
            "source": "test_source",
            "type": "test_type",
            "events": [
                {
                    "event_type": "detection",
                    "description": "Test event"
                }
            ],
            "entity": [
                {
                    "entity_type_key": "ip_address",
                    "value": "192.168.1.1"
                }
            ]
        }

    def tearDown(self):
        self.app_context.pop()

    @patch('soar_api.rest.alerts.records_store')
    @patch('soar_api.rest.alerts.action_logger')
    @patch('soar_api.rest.alerts.create_alert')
    def test_successful_batch_creation(self, mock_create_alert, mock_logger, mock_records_store):
        # Mock successful alert creation
        mock_create_alert.return_value = (
            MagicMock(json={"alert_id": "test_alert_1"}),
            201
        )
        
        # Mock successful database operations
        mock_records_store.create.return_value = {"_id": "test_id"}
        
        test_data = [
            {"tenant": "tenant1", "body": self.valid_alert_data},
            {"tenant": "tenant2", "body": self.valid_alert_data}
        ]
        
        response = create_alerts_batch(test_data)
        response_data = response.get_json()
        
        # Verify response structure
        self.assertIn('message', response_data)
        self.assertIn('results', response_data)
        self.assertEqual(len(response_data['results']['success']), 2)
        self.assertEqual(len(response_data['results']['failed']), 0)
        
        # Verify create_alert was called for each tenant
        self.assertEqual(mock_create_alert.call_count, 2)
        
        # Verify commits were called for each tenant
        mock_records_store.commit.assert_has_calls([
            call('tenant1'),
            call('tenant2')
        ])

    @patch('soar_api.rest.alerts.records_store')
    @patch('soar_api.rest.alerts.action_logger')
    @patch('soar_api.rest.alerts.create_alert')
    def test_partial_failure_batch_creation(self, mock_create_alert, mock_logger, mock_records_store):
        # Mock first alert succeeds, second fails
        mock_create_alert.side_effect = [
            (MagicMock(json={"alert_id": "test_alert_1"}), 201),
            Exception("Test error")
        ]
        
        test_data = [
            {"tenant": "tenant1", "body": self.valid_alert_data},
            {"tenant": "tenant2", "body": self.valid_alert_data}
        ]
        
        response = create_alerts_batch(test_data)
        response_data = response.get_json()
        
        # Verify partial success
        self.assertEqual(len(response_data['results']['success']), 1)
        self.assertEqual(len(response_data['results']['failed']), 1)
        
        # Verify rollback was called for failed tenant
        mock_records_store.rollback.assert_called_once_with('tenant2')

    @patch('soar_api.rest.alerts.records_store')
    @patch('soar_api.rest.alerts.action_logger')
    def test_invalid_input_handling(self, mock_logger, mock_records_store):
        test_data = [
            {"tenant": "tenant1"},  # Missing body
            {"body": self.valid_alert_data},  # Missing tenant
            {}  # Empty data
        ]
        
        response = create_alerts_batch(test_data)
        response_data = response.get_json()
        
        # Verify all attempts failed
        self.assertEqual(len(response_data['results']['success']), 0)
        self.assertEqual(len(response_data['results']['failed']), 3)
        
        # Verify no database operations were attempted
        mock_records_store.create.assert_not_called()
        mock_records_store.commit.assert_not_called()

    @patch('soar_api.rest.alerts.records_store')
    @patch('soar_api.rest.alerts.action_logger')
    @patch('soar_api.rest.alerts.create_alert')
    def test_event_creation(self, mock_create_alert, mock_logger, mock_records_store):
        mock_create_alert.return_value = (
            MagicMock(json={"alert_id": "test_alert_1"}),
            201
        )
        
        # Test data with events
        test_data = [{
            "tenant": "tenant1",
            "body": {
                **self.valid_alert_data,
                "events": [
                    {"event_type": "detection", "description": "Event 1"},
                    {"event_type": "response", "description": "Event 2"}
                ]
            }
        }]
        
        response = create_alerts_batch(test_data)
        response_data = response.get_json()
        
        # Verify events were created
        event_create_calls = [
            call for call in mock_records_store.create.call_args_list
            if call[1]['table'] == 'alert_artifact_event'
        ]
        self.assertEqual(len(event_create_calls), 2)

    @patch('soar_api.rest.alerts.records_store')
    @patch('soar_api.rest.alerts.action_logger')
    @patch('soar_api.rest.alerts.create_alert')
    def test_entity_creation(self, mock_create_alert, mock_logger, mock_records_store):
        mock_create_alert.return_value = (
            MagicMock(json={"alert_id": "test_alert_1"}),
            201
        )
        
        # Mock entity query
        mock_session = MagicMock()
        mock_session.query.return_value.filter.return_value.filter.return_value.all.return_value = []
        mock_records_store.get_db.return_value.session = mock_session
        
        # Test data with entities
        test_data = [{
            "tenant": "tenant1",
            "body": {
                **self.valid_alert_data,
                "entity": [
                    {"entity_type_key": "ip_address", "value": "192.168.1.1"},
                    {"entity_type_key": "hostname", "value": "test-host"}
                ]
            }
        }]
        
        response = create_alerts_batch(test_data)
        response_data = response.get_json()
        
        # Verify entities were created
        entity_create_calls = [
            call for call in mock_records_store.create.call_args_list
            if call[1]['table'] == 'entity'
        ]
        self.assertEqual(len(entity_create_calls), 2)
        
        # Verify entity-alert mappings were created
        mapping_create_calls = [
            call for call in mock_records_store.create.call_args_list
            if call[1]['table'] == 'entity_alert_case_mapping'
        ]
        self.assertEqual(len(mapping_create_calls), 2)

    def test_empty_batch(self):
        response = create_alerts_batch([])
        response_data = response.get_json()
        
        self.assertIn('message', response_data)
        self.assertEqual(len(response_data['results']['success']), 0)
        self.assertEqual(len(response_data['results']['failed']), 0)
