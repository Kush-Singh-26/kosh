import json

files = [
    '.desloppify/subagents/runs/20260316_103712/results/batch-1.json',
    '.desloppify/subagents/runs/20260316_103712/results/batch-10.json',
    '.desloppify/subagents/runs/20260316_103712/results/batch-11.json',
    '.desloppify/subagents/runs/20260316_103712/results/batch-17.json',
]

all_issues = []
for f in files:
    try:
        with open(f, 'r', encoding='utf-8') as fp:
            issues = json.load(fp)
            all_issues.extend(issues)
            print(f'Loaded {len(issues)} issues from {f}')
    except json.JSONDecodeError as e:
        print(f'ERROR loading {f}: {e}')

with open('.desloppify/subagents/runs/20260316_103712/results/combined_issues.json', 'w', encoding='utf-8') as fp:
    json.dump(all_issues, fp, indent=2, ensure_ascii=False)

print(f'\nTotal: {len(all_issues)} issues')
print('Combined JSON written successfully')
