---
title: "2. Neural Nets: Preprocessing data"
description: "Brief description on how data can be processed."
tags: ["Neural-Net"]
date: 2025-03-16
pinned: false
---

# Preprocessing
- It is the process of cleaning, organizing and transforming raw data to improve data quality and ensure faster convergence.
- It is performed before training.

## Preprocessing techniques :

### 1. Standardization (Z-score Normalization)
-  Tranforming / centering input features (data) so that they have :
    - mean (μ) = 0
    - standard deviation (σ) = 1

- $$ X' = \frac{X-μ}{σ} $$
    - X = original feature data
    - X' = Standardized feature

- PyTorch implementation :

```python
import torch

def standardize(data):
    mean = torch.mean(data, dim=0)
    std = torch.std(data, dim=0)
    return (data - mean) / std

data = torch.tensor([[1.0, 2.0], [2.0, 3.0], [3.0, 4.0]])
standardized_data = standardize(data)
print(standardized_data)
```

- Use case : data with Gaussian distribution
- Best for : Linear models, Neural Networks

### 2. Normalize
- transforms / scales data to a fixed range between 0 and 1 (or -1 and 1)

- $$ X' = \frac{X - X_{min}}{X_{max} - X_{min}} $$

- PyTorch implementation on image data

```python
import torchvision.transforms as transforms
from PIL import Image # Library for image manipulation

transform = transforms.Compose([
    transforms.ToTensor(), # convert the data (image) to pytorch tensors
    transforms.Normalize(mean=[0.5], std=[0.5]) # scale to [-1, -1]

    image = Image.open("img.jpg")
    norm_image = transform(image)
])
```
- Use case : Data with varying scales
- Best for : CNNs, Image processing

### Other forms :
- **Imputation** (mean, median, mode)
- **Dropping missing values**
- removing duplicates
- encoding categorical data (one-hot encoding)
