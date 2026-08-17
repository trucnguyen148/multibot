import json
import pandas as pd
import os

input_file = 'study_data_cleaned.json'
output_file = 'study_data_qualitative_wide.csv'

def generate_clean_wide_format():
    if not os.path.exists(input_file):
        print(f"Error: Could not find '{input_file}'. Please ensure it is in the same folder.")
        return

    with open(input_file, 'r') as f:
        data = json.load(f)

    rows = []

    for session in data:
        condition = session.get('condition')
        
        # Pull the database ID if it exists, otherwise note it's missing
        user_id = session.get('user_id', 'ID_Not_Exported') 
        
        pre = session.get('pre_survey_data') or {}
        post = session.get('post_survey_data') or {}
        
        nickname = pre.get('display_name', 'Unknown_Name')
        
        comfort = {
            '1': session.get('stage_1_comfort_score', ''),
            '2': session.get('stage_2_comfort_score', ''),
            '3': session.get('stage_3_comfort_score', '')
        }
        
        transcripts = session.get('chat_transcript') or []
        stage_quotes = {'1': [], '2': [], '3': []}
        
        for msg in transcripts:
            if msg.get('isUser'):
                stage_id = msg.get('stage', '')
                if 'STAGE_1' in stage_id:
                    stage_quotes['1'].append(msg.get('text', ''))
                elif 'STAGE_2' in stage_id:
                    stage_quotes['2'].append(msg.get('text', ''))
                elif 'STAGE_3' in stage_id:
                    stage_quotes['3'].append(msg.get('text', ''))
                    
        # Construct the row with generic coding columns
        row = {
            'user_id': user_id,
            'Nickname': nickname,
            'Condition': condition,
            
            'Stage_1_Quote': " ... ".join(stage_quotes['1']),
            'Stage_1_Comfort': comfort['1'],
            'Stage_1_Codes': '',  # Blank column for team tagging
            
            'Stage_2_Quote': " ... ".join(stage_quotes['2']),
            'Stage_2_Comfort': comfort['2'],
            'Stage_2_Codes': '',  
            
            'Stage_3_Quote': " ... ".join(stage_quotes['3']),
            'Stage_3_Comfort': comfort['3'],
            'Stage_3_Codes': '',  
            
            'Reflection_Quote': post.get('reflection', ''),
            'Reflection_Influence': post.get('reflection_influence', ''),
            'Reflection_Codes': '', 
            
            'Participant_Journey_Notes': '' # Blank space for researcher notes
        }
        
        # # Append Pre-Survey Data
        # for key, value in pre.items():
        #     if key != 'display_name': 
        #         row[f'Pre_{key}'] = value
                
        # Append Post-Survey Data
        for key, value in post.items():
            if key not in ['reflection', 'reflection_influence']:
                row[f'Post_{key}'] = value
            
        rows.append(row)

    # Export
    df_wide = pd.DataFrame(rows)
    df_wide.to_csv(output_file, index=False)
    print(f"Success! File saved as: {output_file}")
    print(f"Total rows: {df_wide.shape[0]}")

if __name__ == "__main__":
    generate_clean_wide_format()