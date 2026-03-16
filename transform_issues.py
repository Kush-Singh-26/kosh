import json
import hashlib

# Map batch files to dimensions
dimension_map = {
    'batch-1.json': 'cross_module_architecture',
    'batch-10.json': 'test_strategy',
    'batch-11.json': 'api_surface_coherence',
    'batch-13.json': 'ai_generated_debt',
    'batch-17.json': 'design_coherence',
}

# Map issue types to identifiers
def make_identifier(issue_type, file, line):
    return f"review::.::holistic::{issue_type}::{file.split('/')[-1].replace('.go', '')}::{line}"

files = [
    ('.desloppify/subagents/runs/20260316_103712/results/batch-1.json', 'cross_module_architecture'),
    ('.desloppify/subagents/runs/20260316_103712/results/batch-10.json', 'test_strategy'),
    ('.desloppify/subagents/runs/20260316_103712/results/batch-11.json', 'api_surface_coherence'),
    ('.desloppify/subagents/runs/20260316_103712/results/batch-17.json', 'design_coherence'),
]

all_issues = []
for f, dimension in files:
    try:
        with open(f, 'r', encoding='utf-8') as fp:
            issues = json.load(fp)
            for issue in issues:
                # Transform to desloppify schema
                transformed = {
                    'dimension': dimension,
                    'identifier': make_identifier(issue['issue_type'], issue['file'], issue['line']),
                    'summary': issue['description'][:200],  # Truncate if too long
                    'file': issue['file'],
                    'line': issue['line'],
                    'code': issue['code'],
                    'issue_type': issue['issue_type'],
                    'description': issue['description'],
                    'suggestion': issue['suggestion'],
                    'evidence': issue.get('evidence', []),
                    'confidence': 'medium',  # Default confidence level
                    'related_files': [issue['file']],
                }
                all_issues.append(transformed)
            print(f'Loaded {len(issues)} issues from {f} -> {dimension}')
    except json.JSONDecodeError as e:
        print(f'ERROR loading {f}: {e}')
    except Exception as e:
        print(f'ERROR processing {f}: {e}')

with open('.desloppify/subagents/runs/20260316_103712/results/issues_transformed.json', 'w', encoding='utf-8') as fp:
    json.dump(all_issues, fp, indent=2, ensure_ascii=False)

print(f'\nTotal: {len(all_issues)} issues')
print('Transformed JSON written successfully')
