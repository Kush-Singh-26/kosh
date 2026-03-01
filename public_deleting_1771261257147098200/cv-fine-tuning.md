---
title: "4. Computer Vision: Fine Tuning Models"
description: "A small guide on how to fine-tune pre-trained models."
tags: ["Computer Vision"]
date: 2025-03-22
pinned: false
---

# Fine Tuning a pre trained model

- Here, instead of freezing all the feature extraction layers, only fist `n` layers will be freezed.
- Last `m` layers will remain trainable along with the classifier.
- Leverage low level features from the pretrained model.
- Provides more potential for final model performance.

## Training Configurations

- Using `dataclasses` module to create classes to organize several training configuration parameter.
- Allows to create data structures for configuration parameters.

```python
@dataclass(frozen=True)  # Creating an immutable dataclass for training configuration
class TrainingConfig:
      batch_size: int = 32  # Number of samples per training batch
      num_epochs: int = 20  # Number of epochs (full passes through dataset)
      learning_rate: float = 1e-4  # Learning rate for optimizer

      log_interval: int = 1  # Interval at which to log training progress
      test_interval: int = 1  # Interval at which to test model performance
      data_root: int = "./"  # Path to dataset storage
      num_workers: int = 5  # Number of worker processes for data loading
      device: str = "cuda"  # Default device for training (CUDA if available)

# Creating an instance of the training configuration
train_config = TrainingConfig()
```

- `@dataclass`
    - python decorator that automatically creates special methods for a class like `__init__()`

- `frozen=True`
    - makes the dataclass immutable, i.e., cannot change its attribute,


## `torchvision.datasets.ImageFolder`

- Helps to load images arranged in folders
- Automatically assigns labels to images based on the folder names.
- Expects the dataset directory to look like :

```
root/
    class1/
        img1.png
        img2.png
        ...
    class2/
        img3.png
        img4.png
        ...
```

```python
torchvision.datasets.ImageFolder(root, transform=None, target_transform=None, loader=<function default_loader>, is_valid_file=None)

train_data = datasets.ImageFolder(root = train_root, transform = train_transforms)
```


## Freezing the first n layers

```python

for param in model.features[:10].parameters():
    param.requires_grad = False
```

## Modifying the final linear layer of classifier

```python
mobilenetv3_model.classifier[3] = nn.Linear(in_features = 1024, out_features = 10, bias = True)
```

> Rest of the training and validation process is same as previous.

> Colab Notebook with the complete implementation can be accessed [here](https://github.com/Kush-Singh-26/Learning-Pytorch/blob/main/Fine_Tuning.ipynb)