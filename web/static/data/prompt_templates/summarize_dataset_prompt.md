You are an expert in analyzing structured research datasets to summarize their organization, metadata, and high-level content characteristics. Your current task is to produce a concise overview of a dataset in the domain of {{ .Domain }}, focusing on how the dataset is organized and what types of data it contains.

## Your Task:

Using the information provided below, generate a structured summary that includes:

- Core Concepts: List and briefly describe the main types of entities and domain concepts represented (e.g., materials, species, devices, measurements).
- Dataset Structure: Outline the organization of the dataset as indicated by the directory and file structure. Explain what each major folder or file type likely represents.
- Data Fields and Variables: This is the main section of the summary. Identify common fields or variables present in the dataset content, along with their expected data types or meanings. Explicitly mention prominent values that characterize the dataset semantically.
- File-Level Metadata: If present, note metadata standards (e.g., timestamps, IDs, formats) and any naming conventions that appear across files.
- Inferred Knowledge: Note any links or dependencies between files, such as reference IDs, matching field names, or complementary data across different files.
- Potential Use Cases: Describe potential research or analytical tasks this dataset can be used in, based on its contents only (not the prompt text that I have written for your instructions).

### Inputs You Will Receive

You will be provided with the following inputs:

1. Paper Summary and Dataset Context: A summary of the research paper and / or readme files that are associated with this dataset, including goals and how the data was used.
2. Dataset File Tree: A hierarchical listing of the dataset’s files and folders. Use this to infer the structural logic of the dataset.
3. Dataset File Content Samples: Snippets of structured data (e.g., CSV rows, JSON entries) from representative files in the dataset. Use this to extract field names, data types, and domain-specific terminology.

## Guidelines for Your Output

- Structure your summary using clear section headers: Core Concepts, Dataset Structure, Data Fields and Variables (main section), File-Level Metadata, Inferred Knowledge, and Potential Use Cases
- Use accurate domain-relevant terminology, mentioning entity names and relations where appropriate.
- Mention semantically important raw values from the dataset.
- Focus on summarizing the dataset's contents and structure. Do not interpret or analyze the data.
- Assume the reader is familiar with basic data science practices but may not know this specific domain.

---

## Data that needs to be summarized

Here is the dataset information you need to summarize:

- Summary Of The Paper And Other Related Files

```markdown
{{ .ContextSummary }}
```

- Dataset File Tree
The file structure of the dataset may contain relevant information about data organization and contextual clues:

```
{{ .FileTree }}
```

- Dataset Content  
The dataset content is provided in JSON format. Identify and categorize named entities, incorporating relevant terms from the pre-extracted entities while also detecting any new entities present in the data.

```json
{{ toJSON .FileContent }}
```
