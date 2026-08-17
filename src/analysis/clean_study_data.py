#!/usr/bin/env python3
"""
Clean study data by:
1. Removing all records with display_name == "Tester" (regardless of state)
2. Keeping only records with current_state == "STATE_COMPLETE" or "STATE_POST_SURVEY"
3. Removing PII fields (user_id, prolific_id, prolific_session_id, study_id)
"""

import json
import sys

def clean_study_data(input_file, output_file):
    """
    Clean the study data JSON file
    
    Args:
        input_file: Path to input JSON file
        output_file: Path to output JSON file
    """
    
    # Load the data
    print(f"Loading data from {input_file}...")
    with open(input_file, 'r') as f:
        data = json.load(f)
    
    print(f"Total records before cleaning: {len(data)}")
    
    # Filter the data
    cleaned_data = []
    
    for record in data:
        # Get display_name (from pre_survey_data)
        display_name = record.get('pre_survey_data', {}).get('display_name')
        current_state = record.get('current_state')
        
        # Rule 1: Remove all records where display_name contains "Tester" (case-insensitive)
        if display_name and "tester" in display_name.lower():
            print(f"  ✗ Removed: Tester user '{display_name}' (state: {current_state})")
            continue
        
        # Rule 2: Keep only STATE_COMPLETE or STATE_POST_SURVEY
        if current_state not in ["STATE_COMPLETE", "STATE_POST_SURVEY"]:
            print(f"  ✗ Removed: {display_name} (state: {current_state})")
            continue
        
        # Rule 3: Remove PII fields from top level
        if 'user_id' in record:
            del record['user_id']
        if 'prolific_id' in record:
            del record['prolific_id']
        if 'prolific_session_id' in record:
            del record['prolific_session_id']
        if 'study_id' in record:
            del record['study_id']
        
        # Rule 4: Remove PII fields from nested pre_survey_data
        if 'pre_survey_data' in record:
            if 'prolific_id' in record['pre_survey_data']:
                del record['pre_survey_data']['prolific_id']
            if 'prolific_session_id' in record['pre_survey_data']:
                del record['pre_survey_data']['prolific_session_id']
            if 'study_id' in record['pre_survey_data']:
                del record['pre_survey_data']['study_id']
        
        # Rule 5: Remove "Tester" references from chat transcripts
        if 'chat_transcript' in record and isinstance(record['chat_transcript'], list):
            for msg in record['chat_transcript']:
                if 'text' in msg and msg['text']:
                    # Replace "Tester" with participant's actual name (or generic placeholder)
                    actual_name = record.get('pre_survey_data', {}).get('display_name', 'Participant')
                    msg['text'] = msg['text'].replace('Tester', actual_name)
                if 'sender' in msg and msg['sender'] == 'Tester':
                    msg['sender'] = actual_name
        
        cleaned_data.append(record)
    
    print(f"\nTotal records after cleaning: {len(cleaned_data)}")
    
    # Summary by condition and state
    condition_counts = {}
    state_counts = {}
    
    for record in cleaned_data:
        condition = record.get('condition')
        state = record.get('current_state')
        
        condition_counts[condition] = condition_counts.get(condition, 0) + 1
        state_counts[state] = state_counts.get(state, 0) + 1
    
    print("\nRecords by condition:")
    for condition in sorted(condition_counts.keys()):
        print(f"  {condition}: {condition_counts[condition]}")
    
    print("\nRecords by state:")
    for state in sorted(state_counts.keys()):
        print(f"  {state}: {state_counts[state]}")
    
    # Verify PII removal
    pii_check = {
        'has_user_id': 0,
        'has_prolific_id': 0,
        'has_prolific_session_id': 0,
        'has_study_id': 0,
        'has_tester': 0
    }
    
    for record in cleaned_data:
        if 'user_id' in record:
            pii_check['has_user_id'] += 1
        if 'prolific_id' in record:
            pii_check['has_prolific_id'] += 1
        if 'prolific_session_id' in record:
            pii_check['has_prolific_session_id'] += 1
        if 'study_id' in record:
            pii_check['has_study_id'] += 1
        
        display_name = record.get('pre_survey_data', {}).get('display_name')
        if display_name and "tester" in display_name.lower():
            pii_check['has_tester'] += 1
        
        if 'pre_survey_data' in record:
            if 'prolific_id' in record['pre_survey_data']:
                pii_check['has_prolific_id'] += 1
            if 'prolific_session_id' in record['pre_survey_data']:
                pii_check['has_prolific_session_id'] += 1
            if 'study_id' in record['pre_survey_data']:
                pii_check['has_study_id'] += 1
    
    print("\nPII Verification:")
    all_clean = True
    for key, count in pii_check.items():
        status = "✓" if count == 0 else "✗"
        print(f"  {status} {key}: {count}")
        if count > 0:
            all_clean = False
    
    if all_clean:
        print("\n✓ All PII successfully removed!")
    
    # Save the cleaned data
    print(f"\nSaving cleaned data to {output_file}...")
    with open(output_file, 'w') as f:
        json.dump(cleaned_data, f, indent=2)
    
    print(f"✓ Done! Cleaned data saved to {output_file}")
    
    return cleaned_data


if __name__ == "__main__":
    # Default paths
    input_file = "study_data.json"
    output_file = "study_data_cleaned.json"
    
    # Allow command-line override
    if len(sys.argv) > 1:
        input_file = sys.argv[1]
    if len(sys.argv) > 2:
        output_file = sys.argv[2]
    
    clean_study_data(input_file, output_file)
